package cli

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"

	"github.com/felixgeelhaar/glossa/platform/internal/cli/config"
	"github.com/felixgeelhaar/glossa/platform/internal/cli/remote"
	"github.com/felixgeelhaar/glossa/platform/internal/cli/snapshot"
)

// keyPattern is the contract's MessageKey.
var keyPattern = regexp.MustCompile(`^[a-z0-9_-]+(\.[a-z0-9_-]+)*$`)

// Push item statuses (the server's, plus "would_*" never: dry runs use
// the same names).
const (
	statusCreated   = "created"
	statusRevised   = "revised"
	statusUpdated   = "updated"
	statusReviewed  = "reviewed"
	statusUnchanged = "unchanged"
	statusFailed    = "failed"
)

type itemError struct {
	Code   string `json:"code"`
	Detail string `json:"detail"`
}

type pushItem struct {
	Key      string     `json:"key"`
	Locale   string     `json:"locale,omitempty"`
	Status   string     `json:"status"`
	Revision int        `json:"revision,omitempty"`
	State    string     `json:"state,omitempty"`
	Error    *itemError `json:"error,omitempty"`
}

type pushJSON struct {
	Schema       string         `json:"schema"`
	DryRun       bool           `json:"dry_run"`
	Source       string         `json:"source"`
	Summary      map[string]int `json:"summary"`
	Messages     []pushItem     `json:"messages"`
	Translations []pushItem     `json:"translations,omitempty"`
}

func runPush(ctx context.Context, inv *invocation, args []string) error {
	fs := inv.flags("push [--dry-run] [--translations]")
	dryRun := fs.Bool("dry-run", false, "report what would change without writing")
	withTranslations := fs.Bool("translations", false, "also import the other locales' catalogs as translations (provenance import)")
	if _, err := inv.parse(fs, args); err != nil {
		return err
	}
	cfg, err := inv.loadConfig()
	if err != nil {
		return err
	}
	local, err := loadLocal(cfg)
	if err != nil {
		return err
	}
	p, err := inv.connectWith(ctx, cfg)
	if err != nil {
		return err
	}
	if p.info.SourceLocale != cfg.SourceLocale {
		return &Error{Exit: ExitUsage, Code: "source_locale_mismatch",
			What:  fmt.Sprintf("glossa.yaml says the source locale is %s, the project's is %s", cfg.SourceLocale, p.info.SourceLocale),
			Where: cfg.Path + " (source_locale)", Fix: "set source_locale: " + p.info.SourceLocale}
	}
	out := pushJSON{Schema: "glossa.cli.push/v1", DryRun: *dryRun, Source: relPath(cfg, local.Locales[0].File)}
	if out.Messages, err = pushMessages(ctx, p, local, *dryRun); err != nil {
		return inv.apiError(err, "push failed")
	}
	if *withTranslations {
		if out.Translations, err = pushTranslations(ctx, p, local, *dryRun); err != nil {
			return inv.apiError(err, "importing translations failed")
		}
	}
	out.Summary = summarize(out.Messages, out.Translations)
	if err := inv.emit(out, func(pr *printer) { printPush(pr, p, out) }); err != nil {
		return err
	}
	if out.Summary[statusFailed] > 0 {
		return silentExit(ExitPartial, "partial_failure")
	}
	return nil
}

// loadLocal reads the local catalogs and explains failures.
func loadLocal(cfg *config.Config) (*snapshot.Snapshot, error) {
	s, err := snapshot.FromLocal(cfg)
	var missing *snapshot.SourceCatalogMissingError
	switch {
	case errors.As(err, &missing):
		return nil, &Error{Exit: ExitUsage, Code: "source_catalog_missing", What: "the source catalog doesn't exist",
			Where: missing.Path, Why: "catalogs.path with source_locale " + cfg.SourceLocale + " points here",
			Fix: "create it (a JSON object of message ID to ICU text), or fix catalogs.path in glossa.yaml"}
	case err != nil:
		return nil, &Error{Exit: ExitUsage, Code: "invalid_catalog", What: "can't read a catalog", Why: err.Error(),
			Fix: "catalogs are JSON objects of message ID to text, flat or nested"}
	}
	return s, nil
}

func relPath(cfg *config.Config, p string) string {
	if r, err := filepath.Rel(cfg.Dir(), p); err == nil {
		return filepath.ToSlash(r)
	}
	return p
}

// localFailure reports a local message push can't send.
func localFailure(key, text string, invalid *snapshot.Invalid) *itemError {
	switch {
	case !keyPattern.MatchString(key) || len(key) > 200:
		return &itemError{Code: "invalid_message_key", Detail: "keys are dotted paths of [a-z0-9_-] segments, e.g. checkout.pay"}
	case invalid != nil:
		return &itemError{Code: "invalid_message", Detail: invalid.Code + ": " + invalid.Detail}
	}
	return nil
}

func pushMessages(ctx context.Context, p *project, local *snapshot.Snapshot, dryRun bool) ([]pushItem, error) {
	syntax := remote.Syntax(p.cfg.SyntaxOrDefault())
	var (
		items []pushItem
		send  []remote.MessageUpsertItem
		index []int
	)
	for _, m := range local.Messages {
		if e := localFailure(m.Key, m.Text, m.Invalid); e != nil {
			items = append(items, pushItem{Key: m.Key, Status: statusFailed, Error: e})
			continue
		}
		index = append(index, len(items))
		items = append(items, pushItem{Key: m.Key})
		send = append(send, remote.MessageUpsertItem{Key: m.Key, Text: m.Text, Syntax: &syntax})
	}
	if dryRun {
		return items, planMessages(ctx, p, local, items, index)
	}
	if len(send) == 0 {
		return items, nil
	}
	res, err := p.client.UpsertMessages(ctx, p.scope, send)
	if err != nil {
		return nil, err
	}
	for i, r := range res {
		it := &items[index[i]]
		it.Status = string(r.Status)
		if r.Message != nil {
			it.Revision = r.Message.SourceRevision
		}
		if r.Error != nil {
			it.Error = &itemError{Code: r.Error.Code, Detail: r.Error.Detail}
		}
	}
	return items, nil
}

// planMessages fills in what a push would do, comparing canonical models.
func planMessages(ctx context.Context, p *project, local *snapshot.Snapshot, items []pushItem, index []int) error {
	server, err := p.client.Messages(ctx, p.scope, remote.MessageFilter{})
	if err != nil {
		return err
	}
	byKey := map[string]remote.Message{}
	for _, m := range server {
		byKey[m.Key] = m
	}
	for _, i := range index {
		it := &items[i]
		lm, _ := local.Message(it.Key)
		sm, ok := byKey[it.Key]
		switch {
		case !ok:
			it.Status = statusCreated
		case snapshot.ModelJSON(lm.Model) != remoteModelJSON(sm):
			it.Status, it.Revision = statusRevised, sm.SourceRevision
		case sm.State == "obsolete":
			it.Status, it.Revision = statusUpdated, sm.SourceRevision
		default:
			it.Status, it.Revision = statusUnchanged, sm.SourceRevision
		}
	}
	return nil
}

func remoteModelJSON(m remote.Message) string {
	model, err := snapshot.DecodeModel(m.Source.Model)
	if err != nil {
		return ""
	}
	return snapshot.ModelJSON(model)
}

func pushTranslations(ctx context.Context, p *project, local *snapshot.Snapshot, dryRun bool) ([]pushItem, error) {
	syntax := remote.Syntax(p.cfg.SyntaxOrDefault())
	var (
		items []pushItem
		send  []remote.TranslationImportItem
		index []int
	)
	for _, l := range local.TargetLocales() {
		trs := local.Translations[l.Code]
		keys := make([]string, 0, len(trs))
		for k := range trs {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		detail := map[string]any{"source": "glossa-cli", "file": relPath(p.cfg, l.File)}
		for _, k := range keys {
			tr := trs[k]
			if e := localFailure(k, tr.Text, tr.Invalid); e != nil {
				items = append(items, pushItem{Key: k, Locale: l.Code, Status: statusFailed, Error: e})
				continue
			}
			index = append(index, len(items))
			items = append(items, pushItem{Key: k, Locale: l.Code})
			send = append(send, remote.TranslationImportItem{Key: k, Locale: l.Code, Text: tr.Text, Syntax: &syntax, OriginDetail: &detail})
		}
	}
	if dryRun {
		return items, planTranslations(ctx, p, local, items, index)
	}
	if len(send) == 0 {
		return items, nil
	}
	res, err := p.client.ImportTranslations(ctx, p.scope, send)
	if err != nil {
		return nil, err
	}
	for i, r := range res {
		applyImportResult(&items[index[i]], r)
	}
	return items, nil
}

func applyImportResult(it *pushItem, r remote.TranslationImportRes) {
	switch {
	case r.Error != nil:
		it.Status = statusFailed
		it.Error = &itemError{Code: r.Error.Code, Detail: r.Error.Detail}
	case r.Status != nil:
		it.Status = string(*r.Status)
	}
	if r.Translation != nil {
		it.Revision, it.State = r.Translation.Revision, string(r.Translation.State)
	}
}

func planTranslations(ctx context.Context, p *project, local *snapshot.Snapshot, items []pushItem, index []int) error {
	server, err := snapshot.FromServer(ctx, p.client, p.scope, p.info.SourceLocale, snapshot.Options{})
	if err != nil {
		return err
	}
	for _, i := range index {
		it := &items[i]
		if _, ok := server.Message(it.Key); !ok {
			it.Status, it.Error = statusFailed, &itemError{Code: "message_not_found", Detail: "no message with this key (push the source first)"}
			continue
		}
		byKey, ok := server.Translations[it.Locale]
		if !ok {
			it.Status, it.Error = statusFailed, &itemError{Code: "locale_not_found", Detail: "the project has no locale " + it.Locale}
			continue
		}
		st, exists := byKey[it.Key]
		lt := local.Translations[it.Locale][it.Key]
		switch {
		case !exists:
			it.Status = statusCreated
		case snapshot.ModelJSON(st.Model) != snapshot.ModelJSON(lt.Model):
			it.Status = statusRevised
		default:
			it.Status, it.State = statusUnchanged, st.State
		}
	}
	return nil
}

func summarize(lists ...[]pushItem) map[string]int {
	s := map[string]int{statusCreated: 0, statusRevised: 0, statusUpdated: 0, statusUnchanged: 0, statusFailed: 0}
	for _, l := range lists {
		for _, it := range l {
			s[it.Status]++
		}
	}
	return s
}

func printPush(pr *printer, p *project, out pushJSON) {
	verb := "Pushed"
	if out.DryRun {
		verb = "Would push"
	}
	pr.line("%s %s from %s to %s (%s)", verb, plural(len(out.Messages), "message", "messages"), out.Source, p.info.Slug, p.cfg.Server)
	printItemCounts(pr, "messages", out.Messages)
	if out.Translations != nil {
		printItemCounts(pr, "translations", out.Translations)
	}
}

func printItemCounts(pr *printer, what string, items []pushItem) {
	counts := summarize(items)
	pr.line("%s %s: %d created · %d revised · %d updated · %d unchanged", pr.pass(), what,
		counts[statusCreated], counts[statusRevised], counts[statusUpdated]+counts[statusReviewed], counts[statusUnchanged])
	if counts[statusFailed] == 0 {
		return
	}
	pr.line("%s %d failed", pr.fail(), counts[statusFailed])
	for _, it := range items {
		if it.Error == nil {
			continue
		}
		label := it.Key
		if it.Locale != "" {
			label = it.Locale + " " + it.Key
		}
		pr.line("  %s  %s: %s", label, it.Error.Code, it.Error.Detail)
	}
}
