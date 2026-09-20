package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/felixgeelhaar/glossa/platform/internal/apiclient"
	"github.com/felixgeelhaar/glossa/platform/internal/cli/config"
	"github.com/felixgeelhaar/glossa/platform/internal/cli/qa"
	"github.com/felixgeelhaar/glossa/platform/internal/cli/remote"
	"github.com/felixgeelhaar/glossa/platform/internal/cli/snapshot"
	"github.com/felixgeelhaar/glossa/platform/internal/cli/terminology"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/bcp47"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/checkpolicy"
)

// maxFindingsPerGroup caps human output; --json has everything.
const maxFindingsPerGroup = 20

type policyJSON struct {
	// RequireComplete is null when every locale is required.
	RequireComplete     []string `json:"require_complete"`
	FailOn              string   `json:"fail_on"`
	MissingTranslations string   `json:"missing_translations"`
	// Source says where the policy came from: "project" when the
	// server's project settings contributed, "local" otherwise.
	Source string `json:"source"`
}

type checkJSON struct {
	Schema string     `json:"schema"`
	Policy policyJSON `json:"policy"`
	qa.Report
}

func runCheck(ctx context.Context, inv *invocation, args []string) error {
	fs := inv.flags("check [--offline] [--terminology] [--require-complete=de,en|none] [--fail-on=error|warning|never]")
	offline := fs.Bool("offline", false, "check the local catalogs instead of the server's project")
	terms := fs.Bool("terminology", false, "also check the translations against the termbase (needs the server)")
	require := fs.String("require-complete", "", "locales that must be complete (comma-separated, or none; default: glossa.yaml's check.require_complete, else the project's check policy)")
	failOn := fs.String("fail-on", "", "lowest severity that fails the check: error, warning or never (default: glossa.yaml's check.fail_on, else the project's check policy)")
	if _, err := inv.parse(fs, args); err != nil {
		return err
	}
	cfg, err := inv.loadConfig()
	if err != nil {
		return err
	}
	if *terms && *offline {
		return usageError(inv.name, "--terminology checks against the server's termbase: drop --offline")
	}
	s, label, checkers, stored, err := inv.checkSnapshot(ctx, cfg, *offline, *terms)
	if err != nil {
		return err
	}
	policy, err := checkPolicy(inv, cfg, stored, *require, *failOn)
	if err != nil {
		return err
	}
	source := "local"
	if stored != nil {
		source = "project"
	}
	report := qa.Run(s, policy, checkers...)
	out := checkJSON{Schema: "glossa.cli.check/v1", Report: report, Policy: policyJSON{
		RequireComplete: policy.RequireComplete, FailOn: string(policy.FailOn),
		MissingTranslations: string(policy.MissingTranslations), Source: source,
	}}
	if err := inv.emit(out, func(p *printer) { printCheck(p, label, report, s.SourceLocale) }); err != nil {
		return err
	}
	if !report.Passed {
		return silentExit(ExitCheckFailed, "check_failed")
	}
	return nil
}

// checkSnapshot reads what the check runs over, and with it the
// project's stored check policy when the server is in reach. Offline
// there is no project to ask, so the policy is nil and the command
// falls back to glossa.yaml and its own default.
func (inv *invocation) checkSnapshot(ctx context.Context, cfg *config.Config, offline, terms bool) (
	*snapshot.Snapshot, string, []qa.Checker, *qa.Policy, error) {
	checkers := qa.Default()
	if offline {
		s, err := loadLocal(cfg)
		return s, "local catalogs", checkers, nil, err
	}
	p, err := inv.connectWith(ctx, cfg)
	if err != nil {
		return nil, "", nil, nil, err
	}
	s, err := snapshot.FromServer(ctx, p.client, p.scope, p.info.SourceLocale, snapshot.Options{})
	if err != nil {
		return nil, "", nil, nil, inv.apiError(err, "can't read the project from the server")
	}
	if terms {
		if checkers, err = inv.terminologyCheckers(ctx, p, s, checkers); err != nil {
			return nil, "", nil, nil, err
		}
	}
	label := fmt.Sprintf("%s on %s", p.info.Slug, cfg.Server)
	return s, label, checkers, projectPolicy(p.info), nil
}

// projectPolicy reads the project's stored check policy. Every project
// has one in a response, so a nil means a server that predates the
// setting — and then the command's own default stands.
func projectPolicy(info remote.Project) *qa.Policy {
	cp := info.Settings.CheckPolicy
	if cp == nil {
		return nil
	}
	p := qa.Policy{FailOn: qa.Severity(cp.FailOn), MissingTranslations: qa.Severity(cp.MissingTranslations)}
	switch cp.RequireComplete {
	case apiclient.CheckPolicyRequireCompleteNone:
		p.RequireComplete = []string{}
	case apiclient.CheckPolicyRequireCompleteListed:
		p.RequireComplete = []string{}
		if cp.Locales != nil {
			p.RequireComplete = append(p.RequireComplete, *cp.Locales...)
		}
	}
	return &p
}

// checkPolicy merges the flags over glossa.yaml over the project's
// stored policy: flags > glossa.yaml > project policy > the built-in
// default. stored is nil offline, and then only the last two apply.
//
// missing_translations has no flag and no glossa.yaml key: whether an
// untranslated key blocks is the project's call, not a local one, or a
// pull request and the terminal would part ways on the one question the
// check exists to answer.
func checkPolicy(inv *invocation, cfg *config.Config, stored *qa.Policy, require, failOn string) (qa.Policy, error) {
	p := qa.Policy{FailOn: qa.Error}
	if stored != nil {
		p = *stored
	}
	if f := orDefault(failOn, cfg.Check.FailOn); f != "" {
		sev, err := checkpolicy.ParseFailOn(f)
		if err != nil {
			return p, usageError(inv.name, "--fail-on must be error, warning or never, not %q", f)
		}
		p.FailOn = sev
	}
	if p.FailOn == "" {
		p.FailOn = qa.Error
	}
	var list []string
	switch {
	case require == "none":
		p.RequireComplete = []string{}
		return p, nil
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
	p.RequireComplete = req
	return p, nil
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

// terminologyCheckers adds the terminology layer: every translation but
// rejected ones, checked against the server's termbase.
func (inv *invocation) terminologyCheckers(ctx context.Context, p *project, s *snapshot.Snapshot, checkers []qa.Checker) ([]qa.Checker, error) {
	var locales []string
	for _, l := range s.TargetLocales() {
		locales = append(locales, l.Code)
	}
	report, err := inv.terminology(ctx, p, terminology.Options{Locales: locales})
	if err != nil {
		return nil, err
	}
	return append(checkers, qa.Precomputed(terminology.CheckName, report.QA())), nil
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
