package cli

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/felixgeelhaar/glossa/platform/internal/cli/catalog"
	"github.com/felixgeelhaar/glossa/platform/internal/cli/release"
	"github.com/felixgeelhaar/glossa/platform/internal/cli/remote"
	"github.com/felixgeelhaar/glossa/platform/internal/cli/snapshot"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/bcp47"
)

// releaseError explains a Release API failure.
func (inv *invocation) releaseError(err error, what string) error {
	if errors.Is(err, release.ErrUnavailable) {
		return &Error{Exit: ExitUsage, Code: "release_api_unavailable", What: what + ": not available yet", Err: err,
			Why: "this glossa-server has no /v1 Release endpoints (publish, promote, rollback, environments, delivery keys) yet",
			Fix: "until then, build-time catalogs come from `glossa pull` (translations as JSON catalogs)"}
	}
	return inv.apiError(err, what)
}

// ── release ─────────────────────────────────────────────────────────

// releaseArgs is a parsed `glossa release` invocation.
type releaseArgs struct {
	action      string // publish, promote, rollback, environments, keys
	environment string
	releaseID   string
	note        string
}

const releaseUsage = `release <action> [flags]

Actions:
  publish       [--environment NAME] [--note TEXT]   publish eligible translations
  promote       <release-id> --to NAME               point an environment at a release
  rollback      --environment NAME [--to RELEASE]    point it back (default: the previous release)
  environments                                       list environments and their releases
  keys                                               list delivery keys`

func parseReleaseArgs(inv *invocation, args []string) (releaseArgs, error) {
	fs := inv.flags(releaseUsage)
	var r releaseArgs
	fs.StringVar(&r.environment, "environment", "", "environment name (publish: default production)")
	to := fs.String("to", "", "promote: the environment; rollback: the release to go back to")
	fs.StringVar(&r.note, "note", "", "publish: a note stored with the release")
	pos, err := inv.parse(fs, args)
	if err != nil {
		return r, err
	}
	if len(pos) == 0 {
		return r, usageError(inv.name, "missing action: publish, promote, rollback, environments or keys")
	}
	r.action, pos = pos[0], pos[1:]
	switch r.action {
	case "publish":
		r.environment = orDefault(r.environment, "production")
		return r, noMore(inv, pos)
	case "promote":
		if len(pos) != 1 || *to == "" {
			return r, usageError(inv.name, "promote takes a release ID and --to <environment>")
		}
		r.releaseID, r.environment = pos[0], *to
		return r, nil
	case "rollback":
		if r.environment == "" {
			return r, usageError(inv.name, "rollback needs --environment <name>")
		}
		r.releaseID = *to
		return r, noMore(inv, pos)
	case "environments", "keys":
		return r, noMore(inv, pos)
	}
	return r, usageError(inv.name, "unknown action %q (publish, promote, rollback, environments, keys)", r.action)
}

func noMore(inv *invocation, pos []string) error {
	if len(pos) > 0 {
		return usageError(inv.name, "unexpected argument %q", pos[0])
	}
	return nil
}

func runRelease(ctx context.Context, inv *invocation, args []string) error {
	r, err := parseReleaseArgs(inv, args)
	if err != nil {
		return err
	}
	p, err := inv.connect(ctx)
	if err != nil {
		return err
	}
	svc, scope := inv.env.releases(), release.Scope{Tenant: p.scope.Tenant, Project: p.scope.Project}
	var result any
	switch r.action {
	case "publish":
		result, err = svc.Publish(ctx, scope, release.PublishRequest{Environment: r.environment, Note: r.note})
	case "promote":
		result, err = svc.Promote(ctx, scope, r.releaseID, r.environment)
	case "rollback":
		result, err = svc.Rollback(ctx, scope, r.environment, r.releaseID)
	case "environments":
		result, err = svc.Environments(ctx, scope)
	case "keys":
		result, err = svc.DeliveryKeys(ctx, scope)
	}
	if err != nil {
		return inv.releaseError(err, "release "+r.action)
	}
	out := map[string]any{"schema": "glossa.cli.release/v1", "action": r.action, "result": result}
	return inv.emit(out, func(pr *printer) { pr.line("%s release %s: %+v", pr.pass(), r.action, result) })
}

// ── pull ────────────────────────────────────────────────────────────

type pulledLocale struct {
	Locale   string `json:"locale"`
	Path     string `json:"path"`
	Messages int    `json:"messages"`
	// Skipped counts translations left out by review state.
	Skipped  map[string]int `json:"skipped"`
	Outdated int            `json:"outdated"`
	Changed  bool           `json:"changed"`
}

type pullJSON struct {
	Schema  string          `json:"schema"`
	States  []string        `json:"states"`
	Locales []pulledLocale  `json:"locales"`
	Release *release.Bundle `json:"release,omitempty"`
}

var reviewStates = []string{"approved", "needs_review", "draft", "rejected"}

func runPull(ctx context.Context, inv *invocation, args []string) error {
	fs := inv.flags("pull [--states approved,needs_review|all] [--locales de,fr] | pull --release <id> [--out dir]")
	states := fs.String("states", "", "review states to pull (comma-separated, or all; default glossa.yaml's pull.states, else approved)")
	locales := fs.String("locales", "", "only these locales (comma-separated)")
	releaseID := fs.String("release", "", "write this release's bundle (manifest.json + a/<sha256>.json) for bundled runtimes")
	outDir := fs.String("out", "glossa-release", "with --release: the bundle directory")
	if _, err := inv.parse(fs, args); err != nil {
		return err
	}
	cfg, err := inv.loadConfig()
	if err != nil {
		return err
	}
	if *releaseID != "" {
		return inv.pullRelease(ctx, *releaseID, cfg.Resolve(*outDir))
	}
	want, err := pullStates(inv, *states, cfg.Pull.States)
	if err != nil {
		return err
	}
	only, err := localeSet(inv, *locales)
	if err != nil {
		return err
	}
	s, _, err := inv.snapshot(ctx, cfg, false, snapshot.Options{})
	if err != nil {
		return err
	}
	style := catalog.Flat
	if cfg.Catalogs.Style == "nested" {
		style = catalog.Nested
	}
	out := pullJSON{Schema: "glossa.cli.pull/v1", States: want, Locales: []pulledLocale{}}
	for _, l := range s.TargetLocales() {
		if only != nil && !only[l.Code] {
			continue
		}
		pl, err := writePulled(s, l.Code, cfg.PullPath(l.Code), want, style)
		if err != nil {
			return &Error{Exit: ExitUsage, Code: "write_failed", What: "can't write a catalog", Where: cfg.PullPath(l.Code), Why: err.Error()}
		}
		pl.Path = relPath(cfg, pl.Path)
		out.Locales = append(out.Locales, pl)
	}
	return inv.emit(out, func(p *printer) {
		for _, l := range out.Locales {
			note := ""
			if !l.Changed {
				note = p.dim(" (unchanged)")
			}
			p.line("%s %s: %s → %s%s", p.pass(), l.Locale, plural(l.Messages, "message", "messages"), l.Path, note)
		}
		if len(out.Locales) == 0 {
			p.line("No locales to pull: the project has only its source locale.")
		}
	})
}

func pullStates(inv *invocation, flagValue string, fromConfig []string) ([]string, error) {
	list := fromConfig
	if flagValue != "" {
		list = strings.Split(flagValue, ",")
	}
	if len(list) == 0 {
		return []string{"approved"}, nil
	}
	if len(list) == 1 && list[0] == "all" {
		return []string{"approved", "needs_review", "draft"}, nil
	}
	for _, s := range list {
		ok := false
		for _, known := range reviewStates {
			ok = ok || s == known
		}
		if !ok {
			return nil, usageError(inv.name, "unknown review state %q (approved, needs_review, draft, rejected, or all)", s)
		}
	}
	return list, nil
}

func localeSet(inv *invocation, flagValue string) (map[string]bool, error) {
	if flagValue == "" {
		return nil, nil
	}
	set := map[string]bool{}
	for _, l := range strings.Split(flagValue, ",") {
		tag, err := bcp47.Parse(strings.TrimSpace(l))
		if err != nil {
			return nil, usageError(inv.name, "%q is not a locale", l)
		}
		set[tag.String()] = true
	}
	return set, nil
}

func writePulled(s *snapshot.Snapshot, locale, path string, states []string, style catalog.Style) (pulledLocale, error) {
	pl := pulledLocale{Locale: locale, Path: path, Skipped: map[string]int{}}
	entries := map[string]string{}
	for key, t := range s.Translations[locale] {
		if !contains(states, t.State) {
			pl.Skipped[t.State]++
			continue
		}
		entries[key] = t.Text
		if t.Outdated {
			pl.Outdated++
		}
	}
	pl.Messages = len(entries)
	changed, err := catalog.Write(path, entries, style)
	pl.Changed = changed
	return pl, err
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func (inv *invocation) pullRelease(ctx context.Context, releaseID, dir string) error {
	p, err := inv.connect(ctx)
	if err != nil {
		return err
	}
	src := inv.env.releases().BundleSource(release.Scope{Tenant: p.scope.Tenant, Project: p.scope.Project}, releaseID)
	b, err := release.WriteBundle(ctx, src, dir)
	if err != nil {
		if errors.Is(err, release.ErrUnavailable) {
			return inv.releaseError(err, "pull --release")
		}
		var ae *remote.APIError
		if errors.As(err, &ae) {
			return inv.apiError(err, "pull --release failed")
		}
		return &Error{Exit: ExitNetwork, Code: "bundle_failed", What: "can't write the release bundle", Where: dir, Why: err.Error()}
	}
	out := pullJSON{Schema: "glossa.cli.pull/v1", Locales: []pulledLocale{}, Release: &b}
	return inv.emit(out, func(pr *printer) {
		pr.line("%s release %s (v%d): %s, %d artifacts → %s", pr.pass(), b.ReleaseID, b.Version,
			strings.Join(b.Locales, ", "), b.Artifacts, filepath.ToSlash(dir))
		pr.line("  %s", pr.dim(fmt.Sprintf("load it as the runtimes' bundled release (runtimes/SPEC.md §3.4), %d bytes", b.Bytes)))
	})
}
