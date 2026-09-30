package cli

import (
	"context"

	"github.com/felixgeelhaar/glossa/platform/internal/cli/capture"
	"github.com/felixgeelhaar/glossa/platform/internal/cli/config"
	"github.com/felixgeelhaar/glossa/platform/internal/cli/qa"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/checkpolicy"
	qualityapp "github.com/felixgeelhaar/glossa/platform/internal/quality/app"
	"github.com/felixgeelhaar/glossa/platform/internal/quality/domain"
	"github.com/felixgeelhaar/glossa/platform/internal/quality/layers"
)

// `glossa capture --check` (RFC 0005 §13 wave 4): capture and check in
// one command.
//
// It is the same check `glossa check` runs — the same layers, the same
// policy, the same document, the same five exit codes — with one layer
// more, the visual one, which only a command that has just had a
// browser open can contribute. Nothing about it is a second check: the
// run is built by the same functions, so a product's CI that captures
// and checks reaches the verdict the terminal reaches.
//
// The command is split in two halves around the capture itself.
// startCheck runs first, before Chrome starts, because the probe pass
// measures against the policy's thresholds and because a run that
// cannot resolve its policy must say so before it captures forty pages,
// not after. finishCheck runs last, when there are captures to promote
// and grade.

// captureCheck is the check half of a `glossa capture --check` run.
type captureCheck struct {
	run       *checkSubject
	policy    checkpolicy.Policy
	overrides checkpolicy.Overrides
	flags     checkFlags
	report    qualityapp.Report
}

// startCheck resolves what the run checks and the policy it grades
// against, exactly as `glossa check` resolves them: the server, else
// the cached document, else the built-in default.
func (inv *invocation) startCheck(ctx context.Context, cfg *config.Config) (*captureCheck, error) {
	f := checkFlags{}
	run, err := inv.checkRun(ctx, cfg, f)
	if err != nil {
		return nil, err
	}
	policy, overrides, err := checkPolicy(inv, cfg, run.policy.Policy, f)
	if err != nil {
		return nil, err
	}
	return &captureCheck{run: run, policy: policy, overrides: overrides, flags: f}, nil
}

// finishCheck grades the project with this run's captures in it.
//
// The visual layer is where the two-sighting rule is applied: what the
// probes measured in each page is sealed into findings, each one
// counted against what the previous capture of the same (route,
// viewport, locale) found, and the fingerprints of this run are left
// behind for the next one.
func (inv *invocation) finishCheck(cfg *config.Config, application string, out *captureJSON) *checkJSON {
	c := out.checked
	project := qa.Project(c.run.snapshot)
	visual, seen := layers.PromoteVisual(
		inv.readSightings(cfg, application), probed(project, out.shots), c.policy.Visual())
	inv.writeSightings(cfg, application, seen)
	c.run.extra = append(c.run.extra, visual)
	checkers, unavailable := c.run.checkers(c.flags)
	c.run.unavailable = append(c.run.unavailable, unavailable...)
	c.report = qa.RunProject(project, c.policy, checkers...)
	doc := checkDocument(c.run, c.report, c.policy, c.overrides, c.flags)
	return &doc
}

// probed is each capture's probe findings under the scope the
// two-sighting rule counts in.
//
// The project identifies them first. A probe ran in a browser and named
// the key it rendered; the catalog message ID a finding's identity is
// hashed over is here, in the snapshot this run read, and putting it in
// the locus is what makes the fingerprint the terminal prints the one the
// server stores (layers.Project.Identify).
func probed(p *layers.Project, shots []capture.Shot) []layers.Probed {
	out := make([]layers.Probed, 0, len(shots))
	for _, s := range shots {
		out = append(out, layers.Probed{
			Scope: layers.VisualScope{
				Route: s.Capture.Route, Width: s.Capture.Viewport.Width,
				Height: s.Capture.Viewport.Height, Locale: s.Capture.Locale,
			},
			Findings: p.Identify(s.Probes),
		})
	}
	return out
}

// probeOptions is what the page measures against: the policy's visual
// thresholds, handed to the probe pass with the capture's options.
//
// This is the whole of "the policy is the source" (RFC 0005 §5.2). The
// constants in runtimes/js/capture/src/probes.ts are defaults for a
// probe called without a driver; every threshold a `glossa capture` run
// measures against comes from here, so there is one place a number is
// changed and no rule is written twice.
func probeOptions(p checkpolicy.Policy) *capture.ProbeOptions {
	v := p.Visual()
	return &capture.ProbeOptions{
		Slack: v.ClipSlackPx, Overlap: v.OverlapPercent,
		Tolerance: v.LineGrowthLines, ByLocale: v.LineGrowthByLocale,
		Max: v.MaxFindingsPerCapture,
	}
}

// printVisual sums the visual layer over the captures, above the check
// itself: how many findings this run measured, and how many of them
// this run is the second sighting of.
func printVisual(p *printer, out *captureJSON) {
	if out.Check == nil {
		return
	}
	var found, promoted int
	for _, f := range out.Check.Findings {
		if f.Layer != domain.LayerVisual {
			continue
		}
		found++
		if !f.Provisional() {
			promoted++
		}
	}
	if found == 0 {
		p.line("%s the visual layer found nothing on %s", p.pass(), plural(len(out.Captures), "capture", "captures"))
		return
	}
	p.line("%s the visual layer found %s on %s, %d seen in the previous capture too",
		p.caution(), plural(found, "finding", "findings"), plural(len(out.Captures), "capture", "captures"), promoted)
	if promoted < found {
		p.line("  %s", p.dim("a finding seen once is a warning no policy can raise: run the capture again to confirm it"))
	}
}
