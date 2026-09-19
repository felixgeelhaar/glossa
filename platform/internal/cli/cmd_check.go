package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/felixgeelhaar/glossa/platform/internal/cli/config"
	"github.com/felixgeelhaar/glossa/platform/internal/cli/qa"
	"github.com/felixgeelhaar/glossa/platform/internal/cli/snapshot"
	"github.com/felixgeelhaar/glossa/platform/internal/cli/terminology"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/bcp47"
)

// maxFindingsPerGroup caps human output; --json has everything.
const maxFindingsPerGroup = 20

type policyJSON struct {
	// RequireComplete is null when every locale is required.
	RequireComplete []string `json:"require_complete"`
	FailOn          string   `json:"fail_on"`
}

type checkJSON struct {
	Schema string     `json:"schema"`
	Policy policyJSON `json:"policy"`
	qa.Report
}

func runCheck(ctx context.Context, inv *invocation, args []string) error {
	fs := inv.flags("check [--offline] [--terminology] [--require-complete=de,en|none] [--fail-on=error|warning]")
	offline := fs.Bool("offline", false, "check the local catalogs instead of the server's project")
	terms := fs.Bool("terminology", false, "also check the translations against the termbase (needs the server)")
	require := fs.String("require-complete", "", "locales that must be complete (comma-separated, or none; default: glossa.yaml's check.require_complete, else all)")
	failOn := fs.String("fail-on", "", "lowest severity that fails the check: error (default) or warning")
	if _, err := inv.parse(fs, args); err != nil {
		return err
	}
	cfg, err := inv.loadConfig()
	if err != nil {
		return err
	}
	policy, err := checkPolicy(inv, cfg, *require, *failOn)
	if err != nil {
		return err
	}
	if *terms && *offline {
		return usageError(inv.name, "--terminology checks against the server's termbase: drop --offline")
	}
	checkers := qa.Default()
	var (
		s     *snapshot.Snapshot
		label string
	)
	if *terms {
		if s, label, checkers, err = inv.terminologySnapshot(ctx, cfg, checkers); err != nil {
			return err
		}
	} else if s, label, err = inv.snapshot(ctx, cfg, *offline, snapshot.Options{}); err != nil {
		return err
	}
	report := qa.Run(s, policy, checkers...)
	out := checkJSON{Schema: "glossa.cli.check/v1", Policy: policyJSON{RequireComplete: policy.RequireComplete, FailOn: string(policy.FailOn)}, Report: report}
	if err := inv.emit(out, func(p *printer) { printCheck(p, label, report, s.SourceLocale) }); err != nil {
		return err
	}
	if !report.Passed {
		return silentExit(ExitCheckFailed, "check_failed")
	}
	return nil
}

// checkPolicy merges flags over glossa.yaml.
func checkPolicy(inv *invocation, cfg *config.Config, require, failOn string) (qa.Policy, error) {
	p := qa.Policy{FailOn: qa.Error}
	switch f := orDefault(failOn, cfg.Check.FailOn); f {
	case "", "error":
	case "warning":
		p.FailOn = qa.Warning
	default:
		return p, usageError(inv.name, "--fail-on must be error or warning, not %q", f)
	}
	var list []string
	switch {
	case require == "none":
		return withRequired(p, []string{}), nil
	case require != "":
		list = strings.Split(require, ",")
	case cfg.Check.RequireComplete != nil:
		list = cfg.Check.RequireComplete
	default:
		return p, nil
	}
	req := []string{}
	for _, l := range list {
		tag, err := bcp47.Parse(strings.TrimSpace(l))
		if err != nil {
			return p, usageError(inv.name, "%q in --require-complete is not a locale", l)
		}
		req = append(req, tag.String())
	}
	return withRequired(p, req), nil
}

func withRequired(p qa.Policy, req []string) qa.Policy {
	p.RequireComplete = req
	return p
}

// snapshot reads the project from the server, or the local catalogs.
func (inv *invocation) snapshot(ctx context.Context, cfg *config.Config, offline bool, opts snapshot.Options) (*snapshot.Snapshot, string, error) {
	if offline {
		s, err := loadLocal(cfg)
		return s, "local catalogs", err
	}
	p, err := inv.connectWith(ctx, cfg)
	if err != nil {
		return nil, "", err
	}
	s, err := snapshot.FromServer(ctx, p.client, p.scope, p.info.SourceLocale, opts)
	if err != nil {
		return nil, "", inv.apiError(err, "can't read the project from the server")
	}
	return s, fmt.Sprintf("%s on %s", p.info.Slug, cfg.Server), nil
}

// terminologySnapshot reads the project from the server and adds the
// terminology layer to checkers: every translation but rejected ones,
// checked against the termbase.
func (inv *invocation) terminologySnapshot(ctx context.Context, cfg *config.Config, checkers []qa.Checker) (*snapshot.Snapshot, string, []qa.Checker, error) {
	p, err := inv.connectWith(ctx, cfg)
	if err != nil {
		return nil, "", nil, err
	}
	s, err := snapshot.FromServer(ctx, p.client, p.scope, p.info.SourceLocale, snapshot.Options{})
	if err != nil {
		return nil, "", nil, inv.apiError(err, "can't read the project from the server")
	}
	var locales []string
	for _, l := range s.TargetLocales() {
		locales = append(locales, l.Code)
	}
	report, err := inv.terminology(ctx, p, terminology.Options{Locales: locales})
	if err != nil {
		return nil, "", nil, err
	}
	label := fmt.Sprintf("%s on %s", p.info.Slug, cfg.Server)
	return s, label, append(checkers, qa.Precomputed(terminology.CheckName, report.QA())), nil
}

func printCheck(p *printer, label string, r qa.Report, source string) {
	mark := func(ok bool) string {
		if ok {
			return p.pass()
		}
		return p.fail()
	}
	byLocale := map[string][]qa.Finding{}
	var structural, args []qa.Finding
	for _, f := range r.Findings {
		switch {
		case f.Check == "structure" && f.Locale == source:
			structural = append(structural, f)
		case f.Check == "arguments":
			args = append(args, f)
		default:
			byLocale[f.Locale] = append(byLocale[f.Locale], f)
		}
	}
	p.line("%s %s discovered (%s)", p.pass(), plural(r.Messages, "message", "messages"), label)
	if len(structural) == 0 {
		p.line("%s message structures valid", p.pass())
	} else {
		p.line("%s %s", p.fail(), plural(len(structural), "invalid message", "invalid messages"))
		printFindings(p, structural, false)
	}
	argErrors := countSeverity(args, qa.Error)
	switch {
	case len(args) == 0:
		p.line("%s arguments valid", p.pass())
	case argErrors == 0:
		p.line("%s arguments: %s", p.caution(), plural(len(args), "warning", "warnings"))
		printFindings(p, args, true)
	default:
		p.line("%s arguments: %s, %s", p.fail(), plural(argErrors, "error", "errors"), plural(len(args)-argErrors, "warning", "warnings"))
		printFindings(p, args, true)
	}
	for _, l := range r.Locales {
		if l.IsSource {
			continue
		}
		fs := byLocale[l.Code]
		delete(byLocale, l.Code)
		errs := countSeverity(fs, qa.Error)
		switch {
		case len(fs) == 0:
			p.line("%s %s complete", mark(true), l.Code)
		case errs == 0:
			p.line("%s %s %s", p.caution(), l.Code, localeSummary(l))
		default:
			p.line("%s %s %s", mark(false), l.Code, localeSummary(l))
		}
		printFindings(p, fs, false)
	}
	for locale, fs := range byLocale { // e.g. a required locale the project lacks
		p.line("%s %s", p.fail(), locale)
		printFindings(p, fs, false)
	}
	p.line("")
	if r.Passed {
		p.line("%s", p.ok("Localization check passed."))
	} else {
		p.line("%s", p.bad(fmt.Sprintf("Localization check failed: %s, %s.", plural(r.Errors, "error", "errors"), plural(r.Warnings, "warning", "warnings"))))
	}
}

func localeSummary(l qa.LocaleReport) string {
	var parts []string
	if l.Missing > 0 {
		parts = append(parts, fmt.Sprintf("%d missing", l.Missing))
	}
	if l.Outdated > 0 {
		parts = append(parts, fmt.Sprintf("%d outdated", l.Outdated))
	}
	if !l.Required && l.Missing > 0 {
		parts = append(parts, "not required")
	}
	if len(parts) == 0 {
		return fmt.Sprintf("%d of %d translated", l.Translated, l.Messages)
	}
	return strings.Join(parts, ", ")
}

func countSeverity(fs []qa.Finding, s qa.Severity) int {
	n := 0
	for _, f := range fs {
		if f.Severity == s {
			n++
		}
	}
	return n
}

func printFindings(p *printer, fs []qa.Finding, withLocale bool) {
	for i, f := range fs {
		if i == maxFindingsPerGroup {
			p.line("  %s", p.dim(fmt.Sprintf("… and %d more (--json lists all)", len(fs)-i)))
			return
		}
		label := f.Key
		if withLocale {
			label = f.Locale + " " + f.Key
		}
		text := f.Message
		if f.Code != qa.CodeMissingTranslation {
			text = f.Code + ": " + f.Message
		}
		sev := p.bad("error")
		if f.Severity == qa.Warning {
			sev = p.warn("warn ")
		}
		p.line("  %s %s  %s", sev, label, text)
	}
}
