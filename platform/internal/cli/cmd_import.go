package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/felixgeelhaar/glossa/platform/internal/cli/remote"
	"github.com/felixgeelhaar/glossa/platform/internal/cli/snapshot"
	"github.com/felixgeelhaar/glossa/platform/internal/cli/v0"
)

type importItem struct {
	Kind     string `json:"kind"` // message or translation
	Key      string `json:"key"`
	Locale   string `json:"locale"`
	Status   string `json:"status"` // created, revised, updated, reviewed, unchanged, planned, skipped, failed
	V0Status string `json:"v0_status,omitempty"`
	State    string `json:"state,omitempty"`
	// Downgraded: approved in v0.3, but the token can't approve here, so
	// it waits for review.
	Downgraded bool       `json:"downgraded,omitempty"`
	Reason     string     `json:"reason,omitempty"`
	Error      *itemError `json:"error,omitempty"`
}

type importJSON struct {
	Schema       string                    `json:"schema"`
	From         string                    `json:"from"`
	Source       map[string]string         `json:"source"`
	DryRun       bool                      `json:"dry_run"`
	LocalesAdded []string                  `json:"locales_added"`
	Summary      map[string]map[string]int `json:"summary"`
	Items        []importItem              `json:"items"`
}

type importFlags struct {
	from, url, project, keyEnv string
	locales                    string
	dryRun                     bool
}

const importUsage = `import --format xliff|json|po|tmx|tbx <file> [--apply | --overwrite] [options]
       glossa import --from v0 --v0-url URL --v0-project SLUG [--v0-key-env GLOSSA_V0_KEY] [--locales de,en] [--dry-run]

An interchange file (--format) goes through the server's import jobs. Without --apply or
--overwrite it is a dry run: every check of a merge, nothing written. Options per format:
  xliff  --syntax mf1 (read other tools' plain units as ICU)
  json   --locale L (default the source: a source catalog) --namespace N --syntax mf1|mf2 --state S
  po     --locale L (default its Language header) --namespace N --state S --plural-variable V
  tmx    --scope project|tenant
  tbx    --scope project|tenant
Exit codes: 0 ok, 1 conflicts or invalid items, 2 usage, 3 refused, 4 the job failed.

--from v0 imports a Glossa v0.3 project through its API (flags --v0-*, --locales, --dry-run).`

func runImport(ctx context.Context, inv *invocation, args []string) error {
	fs := inv.flags(importUsage)
	var f importFlags
	fs.StringVar(&f.from, "from", "", "the system to import from: v0 (Glossa v0.3)")
	fs.StringVar(&f.url, "v0-url", "", "--from v0: the v0.3 API, e.g. https://glossa.example.com/api/v1")
	fs.StringVar(&f.project, "v0-project", "", "--from v0: the v0.3 project slug (default: glossa.yaml's project)")
	fs.StringVar(&f.keyEnv, "v0-key-env", "GLOSSA_V0_KEY", "--from v0: environment variable holding the v0.3 project API key")
	fs.StringVar(&f.locales, "locales", "", "--from v0: only these locales' translations (comma-separated)")
	fs.BoolVar(&f.dryRun, "dry-run", false, "report what the import would do without writing (--format: the default)")
	var ff fileImportFlags
	ff.register(fs)
	pos, err := inv.parse(fs, args)
	if err != nil {
		return err
	}
	switch {
	case f.from != "" && ff.format != "":
		return usageError(inv.name, "--from and --format exclude each other: --format imports a file, --from v0 a Glossa v0.3 project")
	case ff.format != "":
		for _, name := range []string{"v0-url", "v0-project", "v0-key-env", "locales"} {
			if isSet(fs, name) {
				return usageError(inv.name, "--%s belongs to --from v0", name)
			}
		}
		ff.dryRun = f.dryRun
		in, err := parseFileImport(inv, fs, ff, pos)
		if err != nil {
			return err
		}
		return inv.importFile(ctx, in)
	case f.from == "":
		return usageError(inv.name, "import needs --format <xliff|json|po|tmx|tbx> <file>, or --from v0")
	case f.from != "v0":
		return usageError(inv.name, "--from must be v0 (Glossa v0.3); got %q", f.from)
	}
	for _, name := range fileImportFlagNames {
		if isSet(fs, name) {
			return usageError(inv.name, "--%s belongs to --format imports, not --from v0", name)
		}
	}
	if err := noMore(inv, pos); err != nil {
		return err
	}
	if f.url == "" {
		return usageError(inv.name, "--v0-url is required (the v0.3 API, e.g. https://glossa.example.com/api/v1)")
	}
	key := strings.TrimSpace(inv.env.getenv(f.keyEnv))
	if key == "" {
		return &Error{Exit: ExitUsage, Code: "no_v0_key", What: "no v0.3 API key",
			Why: "$" + f.keyEnv + " is empty", Fix: "export " + f.keyEnv + "=glossa_… (a read key of the v0.3 project)"}
	}
	only, err := localeSet(inv, f.locales)
	if err != nil {
		return err
	}
	p, err := inv.connect(ctx)
	if err != nil {
		return err
	}
	f.project = orDefault(f.project, p.cfg.Project)
	src := v0.New(f.url, key, inv.env.HTTP, inv.userAgent())
	plan, err := inv.readV0(ctx, src, f.project, p.info.SourceLocale, only)
	if err != nil {
		return err
	}
	out := importJSON{Schema: "glossa.cli.import/v1", From: "v0", DryRun: f.dryRun, LocalesAdded: []string{},
		Source: map[string]string{"url": src.Base(), "project": f.project}}
	if f.dryRun {
		out.Items = planItems(plan)
	} else if out, err = inv.applyImport(ctx, p, src, f.project, plan, out); err != nil {
		return err
	}
	out.Summary = importSummary(out.Items)
	if err := inv.emit(out, func(pr *printer) { printImport(pr, out) }); err != nil {
		return err
	}
	if out.Summary["message"]["failed"]+out.Summary["translation"]["failed"] > 0 {
		return silentExit(ExitPartial, "partial_failure")
	}
	return nil
}

// readV0 reads every locale of the v0.3 project and plans the import.
func (inv *invocation) readV0(ctx context.Context, src *v0.Client, project, sourceLocale string, only map[string]bool) (v0.Plan, error) {
	fail := func(err error, what string) error {
		var se *v0.StatusError
		e := &Error{Exit: ExitNetwork, Code: "v0_request_failed", What: what, Where: src.Base(), Why: err.Error(), Err: err,
			Fix: "check --v0-url, --v0-project and the v0.3 key"}
		if errors.As(err, &se) && (se.Status == 401 || se.Status == 403) {
			e.Code, e.Fix = "v0_unauthenticated", "use a valid v0.3 project API key (read scope is enough)"
		}
		return e
	}
	codes, err := src.Locales(ctx, project)
	if err != nil {
		return v0.Plan{}, fail(err, "can't list the v0.3 project's locales")
	}
	bundles := map[string]v0.Bundle{}
	for _, code := range codes {
		canon, err := v0.Canonical(code)
		if err != nil {
			return v0.Plan{}, &Error{Exit: ExitUsage, Code: "invalid_locale", What: fmt.Sprintf("v0.3 locale %q is not BCP 47", code)}
		}
		if canon != sourceLocale && only != nil && !only[canon] {
			continue
		}
		b, err := src.Bundle(ctx, project, code)
		if err != nil {
			return v0.Plan{}, fail(err, "can't read v0.3 locale "+code)
		}
		bundles[canon] = b
	}
	plan, err := v0.BuildPlan(sourceLocale, bundles)
	if err != nil {
		return v0.Plan{}, &Error{Exit: ExitUsage, Code: "source_locale_missing", What: "can't import this v0.3 project", Why: err.Error(),
			Fix: "import into a project whose source locale the v0.3 project has"}
	}
	return plan, nil
}

func planItems(plan v0.Plan) []importItem {
	var items []importItem
	for _, m := range plan.Messages {
		it := importItem{Kind: "message", Key: m.Key, Locale: plan.SourceLocale, Status: "planned"}
		markInvalid(&it, m.Invalid)
		items = append(items, it)
	}
	for _, t := range plan.Translations {
		it := importItem{Kind: "translation", Key: t.Key, Locale: t.Locale, Status: "planned", V0Status: t.V0Status, State: t.State}
		markInvalid(&it, t.Invalid)
		items = append(items, it)
	}
	for _, s := range plan.Skipped {
		kind := "translation"
		if s.Locale == plan.SourceLocale {
			kind = "message"
		}
		items = append(items, importItem{Kind: kind, Key: s.Key, Locale: s.Locale, Status: "skipped", Reason: s.Reason})
	}
	return items
}

func markInvalid(it *importItem, invalid *snapshot.Invalid) {
	if invalid != nil {
		it.Status = statusFailed
		it.Error = &itemError{Code: "invalid_message", Detail: invalid.Code + ": " + invalid.Detail}
	}
}

// applyImport writes the plan: locales, then messages, then translations.
func (inv *invocation) applyImport(ctx context.Context, p *project, src *v0.Client, project string, plan v0.Plan, out importJSON) (importJSON, error) {
	for _, l := range plan.Locales {
		_, created, err := p.client.AddLocale(ctx, p.scope, l)
		if err != nil {
			return out, inv.apiError(err, "can't add locale "+l)
		}
		if created {
			out.LocalesAdded = append(out.LocalesAdded, l)
		}
	}
	msgItems, imported, err := importMessages(ctx, p, plan)
	if err != nil {
		return out, inv.apiError(err, "importing messages failed")
	}
	trItems, err := importTranslations(ctx, p, src.Base(), project, plan, imported)
	if err != nil {
		return out, inv.apiError(err, "importing translations failed")
	}
	out.Items = append(append(msgItems, trItems...), skippedItems(plan)...)
	return out, nil
}

func skippedItems(plan v0.Plan) []importItem {
	return planItems(v0.Plan{SourceLocale: plan.SourceLocale, Skipped: plan.Skipped})
}

func importMessages(ctx context.Context, p *project, plan v0.Plan) ([]importItem, map[string]bool, error) {
	syntax := remote.Syntax("mf1")
	var (
		items []importItem
		send  []remote.MessageUpsertItem
		index []int
	)
	for _, m := range plan.Messages {
		it := importItem{Kind: "message", Key: m.Key, Locale: plan.SourceLocale}
		if e := localFailure(m.Key, m.Text, m.Invalid); e != nil {
			it.Status, it.Error = statusFailed, e
			items = append(items, it)
			continue
		}
		index = append(index, len(items))
		items = append(items, it)
		send = append(send, remote.MessageUpsertItem{Key: m.Key, Text: m.Text, Syntax: &syntax})
	}
	imported := map[string]bool{}
	if len(send) == 0 {
		return items, imported, nil
	}
	res, err := p.client.UpsertMessages(ctx, p.scope, send)
	if err != nil {
		return nil, nil, err
	}
	for i, r := range res {
		it := &items[index[i]]
		it.Status = string(r.Status)
		if r.Error != nil {
			it.Error = &itemError{Code: r.Error.Code, Detail: r.Error.Detail}
			continue
		}
		imported[it.Key] = true
	}
	return items, imported, nil
}

// existingTranslations reads the server's translations of keys as
// locale → key → (canonical model, state), with the bulk listing.
func existingTranslations(ctx context.Context, p *project, keys map[string]bool) (map[string]map[string][2]string, error) {
	locales, err := p.client.Locales(ctx, p.scope)
	if err != nil {
		return nil, err
	}
	var targets []string
	for _, l := range locales {
		if !l.IsSource {
			targets = append(targets, l.Code)
		}
	}
	trs, err := p.client.ProjectTranslations(ctx, p.scope, targets, remote.TranslationFilter{})
	if err != nil {
		return nil, err
	}
	out := map[string]map[string][2]string{}
	for _, t := range trs {
		if !keys[t.Key] {
			continue
		}
		model, err := snapshot.DecodeModel(t.Model)
		if err != nil {
			continue
		}
		if out[t.Locale] == nil {
			out[t.Locale] = map[string][2]string{}
		}
		out[t.Locale][t.Key] = [2]string{snapshot.ModelJSON(model), string(t.State)}
	}
	return out, nil
}

// importTranslations writes the v0.3 values. A value the server already
// has (same canonical model) is left alone, so a re-run changes nothing
// and never undoes a review made since the first run.
func importTranslations(ctx context.Context, p *project, base, project string, plan v0.Plan, imported map[string]bool) ([]importItem, error) {
	syntax := remote.Syntax("mf1")
	existing, err := existingTranslations(ctx, p, imported)
	if err != nil {
		return nil, err
	}
	var (
		items []importItem
		send  []remote.TranslationImportItem
		index []int
	)
	for _, t := range plan.Translations {
		it := importItem{Kind: "translation", Key: t.Key, Locale: t.Locale, V0Status: t.V0Status}
		switch e := localFailure(t.Key, t.Text, t.Invalid); {
		case e != nil:
			it.Status, it.Error = statusFailed, e
		case !imported[t.Key]:
			it.Status, it.Reason = "skipped", "its message wasn't imported"
		default:
			model, _, _ := snapshot.Parse("mf1", t.Text, t.Locale)
			if have, ok := existing[t.Locale][t.Key]; ok && have[0] == snapshot.ModelJSON(model) {
				it.Status, it.State = statusUnchanged, have[1]
			}
		}
		if it.Status != "" {
			items = append(items, it)
			continue
		}
		state := remote.ReviewState(t.State)
		detail := map[string]any{"source": "glossa-v0.3", "url": base, "project": project, "status": orDefault(t.V0Status, "none")}
		index = append(index, len(items))
		items = append(items, it)
		send = append(send, remote.TranslationImportItem{Key: t.Key, Locale: t.Locale, Text: t.Text, Syntax: &syntax,
			State: &state, OriginDetail: &detail})
	}
	if len(send) == 0 {
		return items, nil
	}
	res, err := p.client.ImportTranslations(ctx, p.scope, send)
	if err != nil {
		return nil, err
	}
	// A token can't approve where review is required: those wait for a
	// reviewer instead of failing.
	var retry []int
	for i, r := range res {
		if r.Error != nil && r.Error.Code == "review_forbidden" {
			retry = append(retry, i)
			continue
		}
		applyImport(&items[index[i]], r)
	}
	if len(retry) > 0 {
		again := make([]remote.TranslationImportItem, len(retry))
		review := remote.ReviewState("needs_review")
		for j, i := range retry {
			again[j] = send[i]
			again[j].State = &review
		}
		res2, err := p.client.ImportTranslations(ctx, p.scope, again)
		if err != nil {
			return nil, err
		}
		for j, r := range res2 {
			it := &items[index[retry[j]]]
			applyImport(it, r)
			it.Downgraded = r.Error == nil
		}
	}
	return items, nil
}

func applyImport(it *importItem, r remote.TranslationImportRes) {
	var pi pushItem
	applyImportResult(&pi, r)
	it.Status, it.State = pi.Status, pi.State
	it.Error = pi.Error
}

func importSummary(items []importItem) map[string]map[string]int {
	s := map[string]map[string]int{"message": {}, "translation": {}}
	for _, it := range items {
		s[it.Kind][it.Status]++
	}
	return s
}

func printImport(p *printer, out importJSON) {
	verb := "Imported"
	if out.DryRun {
		verb = "Would import"
	}
	p.line("%s from Glossa v0.3 %s (%s)", verb, out.Source["project"], out.Source["url"])
	if len(out.LocalesAdded) > 0 {
		p.line("%s added locales %s", p.pass(), strings.Join(out.LocalesAdded, ", "))
	}
	for _, kind := range []string{"message", "translation"} {
		counts := out.Summary[kind]
		var parts []string
		for _, st := range []string{"planned", "created", "revised", "updated", "reviewed", "unchanged", "skipped", "failed"} {
			if counts[st] > 0 {
				parts = append(parts, fmt.Sprintf("%d %s", counts[st], st))
			}
		}
		if len(parts) == 0 {
			parts = []string{"none"}
		}
		mark := p.pass()
		if counts["failed"] > 0 {
			mark = p.fail()
		}
		p.line("%s %ss: %s", mark, kind, strings.Join(parts, " · "))
	}
	downgraded := 0
	for _, it := range out.Items {
		if it.Downgraded {
			downgraded++
		}
		if it.Error != nil {
			p.line("  %s %s %s: %s", it.Locale, it.Key, it.Error.Code, it.Error.Detail)
		}
	}
	if downgraded > 0 {
		p.line("%s %d approved in v0.3 now wait for review: API tokens can't approve (a reviewer approves them in Studio)", p.caution(), downgraded)
	}
}
