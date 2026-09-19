package cli

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/felixgeelhaar/glossa/platform/internal/cli/remote"
)

// ── output shapes (cmd/glossa/README.md, "JSON output") ─────────────

type translateFilterJSON struct {
	Namespace string `json:"namespace,omitempty"`
	KeyPrefix string `json:"key_prefix,omitempty"`
	Missing   bool   `json:"missing"`
	Outdated  bool   `json:"outdated"`
}

// translatePlanJSON is what a dry run would queue in one locale.
type translatePlanJSON struct {
	Locale string   `json:"locale"`
	Queue  int      `json:"queue"`
	Keys   []string `json:"keys"`
}

// refusalJSON is a reason jobs will do little: consent off, no budget,
// no provider.
type refusalJSON struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Fix     string `json:"fix"`
}

type fillJSON struct {
	ID           string         `json:"id"`
	Locales      []string       `json:"locales"`
	Keys         int            `json:"keys,omitempty"`
	JobsCreated  int            `json:"jobs_created"`
	JobsExisting int            `json:"jobs_existing"`
	Skipped      map[string]int `json:"skipped"`
	JobStates    map[string]int `json:"job_states"`
	Warnings     []string       `json:"warnings"`
}

type failedJobJSON struct {
	ID          string `json:"id"`
	Key         string `json:"key"`
	Locale      string `json:"locale"`
	State       string `json:"state"`
	FailureCode string `json:"failure_code,omitempty"`
	Error       string `json:"error,omitempty"`
}

type waitJSON struct {
	ElapsedMS int64           `json:"elapsed_ms"`
	JobStates map[string]int  `json:"job_states"`
	Failed    []failedJobJSON `json:"failed"`
}

type translateJSON struct {
	Schema   string              `json:"schema"`
	DryRun   bool                `json:"dry_run"`
	Locales  []string            `json:"locales"`
	Filter   translateFilterJSON `json:"filter"`
	Plan     []translatePlanJSON `json:"plan"`
	Skipped  map[string]int      `json:"skipped"`
	Refusals []refusalJSON       `json:"refusals"`
	Fills    []fillJSON          `json:"fills"`
	// Wait is null without --wait.
	Wait *waitJSON `json:"wait"`
}

const translateUsage = `translate --locale L [--locale L]... [--namespace NS] [--key-prefix P] [--missing] [--outdated]
                 [--dry-run] [--wait [--timeout 10m]]

Fills locales with AI suggestions (RFC 0003): one job per message missing (default) or outdated
(--outdated; both flags: both) in each locale. Messages in sensitive namespaces are never sent.
Suggestions are applied or queued for review by the project's routing policy
(` + "`glossa review list`" + ` shows the queue).

--dry-run reports what would be queued, and why jobs would do little (consent off, no budget,
no provider): it exits 1 when there is such a refusal. --wait polls the jobs until they finish
and exits 4 when some failed.`

type translateArgs struct {
	locales                         listFlag
	namespace, keyPrefix            string
	missing, outdated, dryRun, wait bool
	timeout, pollInterval           time.Duration
	idempotencyKey                  string
}

func parseTranslateArgs(inv *invocation, args []string) (translateArgs, error) {
	fs := inv.flags(translateUsage)
	var a translateArgs
	fs.Var(&a.locales, "locale", "a target locale (repeatable, or comma-separated)")
	fs.StringVar(&a.namespace, "namespace", "", "only messages in this namespace")
	fs.StringVar(&a.keyPrefix, "key-prefix", "", "only messages whose key starts with this")
	fs.BoolVar(&a.missing, "missing", false, "translate messages without a translation (the default)")
	fs.BoolVar(&a.outdated, "outdated", false, "translate messages whose translation is outdated")
	fs.BoolVar(&a.dryRun, "dry-run", false, "report what would be queued; queue nothing")
	fs.BoolVar(&a.wait, "wait", false, "wait for the jobs to finish")
	fs.DurationVar(&a.timeout, "timeout", 10*time.Minute, "with --wait: give up after this long")
	fs.DurationVar(&a.pollInterval, "poll-interval", 2*time.Second, "with --wait: how often to ask for progress")
	fs.StringVar(&a.idempotencyKey, "idempotency-key", "", "the Idempotency-Key (default: a new one per invocation)")
	pos, err := inv.parse(fs, args)
	if err != nil {
		return a, err
	}
	if err := noMore(inv, pos); err != nil {
		return a, err
	}
	if len(a.locales) == 0 {
		return a, usageError(inv.name, "translate needs --locale <locale> (repeatable)")
	}
	if !a.missing && !a.outdated {
		a.missing = true
	}
	if a.timeout <= 0 || a.pollInterval <= 0 {
		return a, usageError(inv.name, "--timeout and --poll-interval must be positive")
	}
	if a.idempotencyKey != "" && !idempotencyKeyPattern.MatchString(a.idempotencyKey) {
		return a, usageError(inv.name, "--idempotency-key must be 1-255 printable ASCII characters without spaces")
	}
	if a.dryRun && a.wait {
		return a, usageError(inv.name, "--dry-run queues nothing to --wait for")
	}
	return a, nil
}

func runTranslate(ctx context.Context, inv *invocation, args []string) error {
	a, err := parseTranslateArgs(inv, args)
	if err != nil {
		return err
	}
	locales, err := normalizeLocales(inv, "--locale", a.locales)
	if err != nil {
		return err
	}
	p, err := inv.connect(ctx)
	if err != nil {
		return err
	}
	if err := inv.checkTargetLocales(ctx, p, locales); err != nil {
		return err
	}
	out := translateJSON{Schema: "glossa.cli.translate/v1", DryRun: a.dryRun, Locales: locales,
		Filter: translateFilterJSON{Namespace: a.namespace, KeyPrefix: a.keyPrefix, Missing: a.missing, Outdated: a.outdated},
		Plan:   []translatePlanJSON{}, Skipped: map[string]int{}, Refusals: []refusalJSON{}, Fills: []fillJSON{}}
	if a.dryRun {
		return inv.translateDryRun(ctx, p, a, out)
	}
	return inv.translateFill(ctx, p, a, out)
}

// checkTargetLocales refuses locales the project lacks and its source.
func (inv *invocation) checkTargetLocales(ctx context.Context, p *project, locales []string) error {
	ls, err := p.client.Locales(ctx, p.scope)
	if err != nil {
		return inv.apiError(err, "can't list locales")
	}
	for _, l := range locales {
		if l == p.info.SourceLocale {
			return usageError(inv.name, "%s is the source locale; translate into the others", l)
		}
		if !slices.ContainsFunc(ls, func(pl remote.ProjectLocale) bool { return pl.Code == l }) {
			return &Error{Exit: ExitUsage, Code: "locale_not_found", What: "the project has no locale " + l,
				Fix: "add it in Studio first, or check --locale (`glossa locales` lists them)"}
		}
	}
	return nil
}

// refusals explains what keeps jobs from reaching a provider, from the
// fill's warning codes.
func refusals(codes []string) []refusalJSON {
	out := []refusalJSON{}
	for _, c := range codes {
		r := refusalJSON{Code: c}
		switch c {
		case "provider_consent_off":
			r.Message = "sending text to AI providers is off for the tenant: only exact translation-memory matches are reused, the other jobs fail (provider_consent)"
			r.Fix = "a tenant admin turns on provider consent in Studio (AI settings); `glossa ai status` shows it"
		case "no_budget":
			r.Message = "the tenant's monthly AI budget is 0: no provider calls are made"
			r.Fix = "a tenant admin sets a monthly budget in Studio (AI settings)"
		case "budget_exhausted":
			r.Message = "this month's AI budget is spent: provider calls are refused (budget_exceeded)"
			r.Fix = "wait for next month, or a tenant admin raises the budget; `glossa ai status` shows the spend"
		case "no_provider":
			r.Message = "no AI provider is configured and enabled: jobs fail (no_route)"
			r.Fix = "a tenant admin adds a provider with its API key in Studio (AI settings)"
		default:
			r.Message = c
		}
		out = append(out, r)
	}
	return out
}

// preflight reads why jobs would do little, as the fill would warn.
func (inv *invocation) preflight(ctx context.Context, p *project) ([]string, error) {
	settings, err := p.client.AISettings(ctx, p.scope.Tenant)
	if err != nil {
		return nil, inv.m2Error(err, "can't read the AI settings")
	}
	budget, err := p.client.AIBudget(ctx, p.scope.Tenant)
	if err != nil {
		return nil, inv.m2Error(err, "can't read the AI budget")
	}
	providers, err := p.client.AIProviders(ctx, p.scope.Tenant)
	if err != nil {
		return nil, inv.m2Error(err, "can't list the AI providers")
	}
	var codes []string
	if !settings.ProviderConsent {
		codes = append(codes, "provider_consent_off")
	}
	switch {
	case settings.MonthlyBudgetMicroUsd == 0:
		codes = append(codes, "no_budget")
	case budget.RemainingMicroUsd <= 0:
		codes = append(codes, "budget_exhausted")
	}
	if !slices.ContainsFunc(providers, func(pr remote.AIProvider) bool { return pr.Enabled }) {
		codes = append(codes, "no_provider")
	}
	return codes, nil
}

func (inv *invocation) translateDryRun(ctx context.Context, p *project, a translateArgs, out translateJSON) error {
	ps, err := p.client.ProjectAISettings(ctx, p.scope)
	if err != nil {
		return inv.m2Error(err, "can't read the project's AI settings")
	}
	for _, l := range out.Locales {
		msgs, err := inv.fillCandidates(ctx, p, a, l)
		if err != nil {
			return err
		}
		plan := translatePlanJSON{Locale: l, Keys: []string{}}
		for _, m := range msgs {
			if slices.Contains(ps.NamespaceTags[m.Namespace], "sensitive") {
				out.Skipped["sensitive"]++
				continue
			}
			plan.Keys = append(plan.Keys, m.Key)
		}
		plan.Queue = len(plan.Keys)
		out.Plan = append(out.Plan, plan)
	}
	codes, err := inv.preflight(ctx, p)
	if err != nil {
		return err
	}
	out.Refusals = refusals(codes)
	if err := inv.emit(out, func(pr *printer) { printTranslatePlan(pr, out) }); err != nil {
		return err
	}
	if len(out.Refusals) > 0 {
		return silentExit(ExitCheckFailed, "translate_refused")
	}
	return nil
}

// fillCandidates are the messages a fill covers in locale, by key.
func (inv *invocation) fillCandidates(ctx context.Context, p *project, a translateArgs, locale string) ([]remote.Message, error) {
	var filters []remote.MessageFilter
	if a.missing {
		filters = append(filters, remote.MessageFilter{State: "active", Namespace: a.namespace, KeyPrefix: a.keyPrefix, MissingIn: locale})
	}
	if a.outdated {
		filters = append(filters, remote.MessageFilter{State: "active", Namespace: a.namespace, KeyPrefix: a.keyPrefix, OutdatedIn: locale})
	}
	seen := map[string]bool{}
	var out []remote.Message
	for _, f := range filters {
		msgs, err := p.client.Messages(ctx, p.scope, f)
		if err != nil {
			return nil, inv.apiError(err, "can't list messages")
		}
		for _, m := range msgs {
			if !seen[m.Key] {
				seen[m.Key] = true
				out = append(out, m)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

func printTranslatePlan(pr *printer, out translateJSON) {
	total := 0
	for _, pl := range out.Plan {
		total += pl.Queue
	}
	pr.line("Dry run: %s would be queued %s", plural(total, "AI job", "AI jobs"), pr.dim("(nothing was queued)"))
	for _, pl := range out.Plan {
		pr.line("  %s  %s", pl.Locale, plural(pl.Queue, "message", "messages"))
		for i, k := range pl.Keys {
			if i == maxFindingsPerGroup {
				pr.line("    %s", pr.dim(fmt.Sprintf("… and %d more (--json lists all)", len(pl.Keys)-i)))
				break
			}
			pr.line("    %s", k)
		}
	}
	if n := out.Skipped["sensitive"]; n > 0 {
		pr.line("  %s", pr.dim(fmt.Sprintf("%s skipped: sensitive namespaces are never sent to a provider", plural(n, "message", "messages"))))
	}
	printRefusals(pr, out.Refusals)
}

func printRefusals(pr *printer, rs []refusalJSON) {
	for _, r := range rs {
		pr.line("%s %s", pr.caution(), r.Message)
		pr.line("  %s %s", pr.dim("fix:"), r.Fix)
	}
}

func toFillJSON(f remote.AIFill) fillJSON {
	out := fillJSON{ID: f.Id, Locales: f.Locales, JobsCreated: f.JobsCreated, JobsExisting: f.JobsExisting,
		Skipped: f.Skipped, JobStates: f.JobStates, Warnings: nonNilList(f.Warnings)}
	if f.Keys != nil {
		out.Keys = len(*f.Keys)
	}
	if out.Skipped == nil {
		out.Skipped = map[string]int{}
	}
	if out.JobStates == nil {
		out.JobStates = map[string]int{}
	}
	return out
}

// fillRequests are the fills to create: one for missing (and outdated)
// messages across the locales; for outdated only, one per locale and 500
// outdated keys, since listed keys are filled when missing or outdated.
func (inv *invocation) fillRequests(ctx context.Context, p *project, a translateArgs, locales []string) ([]remote.CreateAIFill, error) {
	if a.missing {
		body := remote.CreateAIFill{Locales: locales, Namespace: optionalStr(a.namespace), KeyPrefix: optionalStr(a.keyPrefix)}
		if a.outdated {
			body.IncludeOutdated = &a.outdated
		}
		return []remote.CreateAIFill{body}, nil
	}
	var out []remote.CreateAIFill
	for _, l := range locales {
		msgs, err := inv.fillCandidates(ctx, p, a, l)
		if err != nil {
			return nil, err
		}
		for start := 0; start < len(msgs); start += remote.MaxBatch {
			keys := make([]string, 0, remote.MaxBatch)
			for _, m := range msgs[start:min(start+remote.MaxBatch, len(msgs))] {
				keys = append(keys, m.Key)
			}
			out = append(out, remote.CreateAIFill{Locales: []string{l}, Keys: &keys})
		}
	}
	return out, nil
}

func (inv *invocation) translateFill(ctx context.Context, p *project, a translateArgs, out translateJSON) error {
	bodies, err := inv.fillRequests(ctx, p, a, out.Locales)
	if err != nil {
		return err
	}
	key := orDefault(a.idempotencyKey, newIdempotencyKey())
	var warnings []string
	for i, body := range bodies {
		k := key
		if i > 0 {
			k = fmt.Sprintf("%s-%d", key, i)
		}
		f, err := p.client.CreateAIFill(ctx, p.scope, body, k)
		if err != nil {
			return inv.m2Error(err, "can't request the AI fill")
		}
		fj := toFillJSON(f)
		out.Fills = append(out.Fills, fj)
		for reason, n := range fj.Skipped {
			out.Skipped[reason] += n
		}
		for _, w := range fj.Warnings {
			if !contains(warnings, w) {
				warnings = append(warnings, w)
			}
		}
	}
	out.Refusals = refusals(warnings)
	if !a.wait {
		return inv.emit(out, func(pr *printer) { printFills(pr, out) })
	}
	if !inv.json {
		printFills(inv.out, out)
	}
	w, err := inv.waitForFills(ctx, p, &out, a)
	if err != nil {
		return err
	}
	out.Wait = w
	if err := inv.emit(out, func(pr *printer) { printWait(pr, *w, out.Locales) }); err != nil {
		return err
	}
	if len(w.Failed) > 0 {
		return silentExit(ExitPartial, "jobs_failed")
	}
	return nil
}

func printFills(pr *printer, out translateJSON) {
	created, existing := 0, 0
	for _, f := range out.Fills {
		created += f.JobsCreated
		existing += f.JobsExisting
	}
	if len(out.Fills) == 0 {
		pr.line("%s Nothing to translate in %s.", pr.pass(), strings.Join(out.Locales, ", "))
		return
	}
	extra := ""
	if existing > 0 {
		extra = fmt.Sprintf(" (%d already queued or done)", existing)
	}
	ids := make([]string, len(out.Fills))
	for i, f := range out.Fills {
		ids[i] = f.ID
	}
	pr.line("%s Queued %s for %s%s %s", pr.pass(), plural(created, "AI job", "AI jobs"), strings.Join(out.Locales, ", "), extra,
		pr.dim("(fill "+strings.Join(ids, ", ")+")"))
	var skipped []string
	for reason, n := range out.Skipped {
		skipped = append(skipped, fmt.Sprintf("%d %s", n, strings.ReplaceAll(reason, "_", " ")))
	}
	sort.Strings(skipped)
	if len(skipped) > 0 {
		pr.line("  %s", pr.dim("skipped: "+strings.Join(skipped, ", ")))
	}
	printRefusals(pr, out.Refusals)
}

// finalJobStates are the states a job doesn't leave.
func finalJobState(s string) bool { return s != "queued" && s != "running" }

// waitForFills polls the fills until their jobs are final or the
// timeout passes, printing progress to stderr.
func (inv *invocation) waitForFills(ctx context.Context, p *project, out *translateJSON, a translateArgs) (*waitJSON, error) {
	start := time.Now()
	deadline := start.Add(a.timeout)
	progress := !inv.json && !inv.quiet
	last := ""
	for {
		states := map[string]int{}
		for i := range out.Fills {
			f, err := p.client.AIFill(ctx, p.scope.Tenant, out.Fills[i].ID)
			if err != nil {
				return nil, inv.m2Error(err, "can't read the fill's progress")
			}
			out.Fills[i] = toFillJSON(f)
			for s, n := range out.Fills[i].JobStates {
				states[s] += n
			}
		}
		total, final := 0, 0
		for s, n := range states {
			total += n
			if finalJobState(s) {
				final += n
			}
		}
		if progress {
			line := fmt.Sprintf("  %d/%d jobs finished (%d running, %d queued)", final, total, states["running"], states["queued"])
			if line != last {
				if inv.env.ColorOutput {
					fmt.Fprintf(inv.env.Stderr, "\r%s", line)
				} else {
					fmt.Fprintln(inv.env.Stderr, line)
				}
				last = line
			}
		}
		if final == total {
			if progress && inv.env.ColorOutput {
				fmt.Fprintln(inv.env.Stderr)
			}
			w := &waitJSON{ElapsedMS: time.Since(start).Milliseconds(), JobStates: states, Failed: []failedJobJSON{}}
			failed, err := inv.failedJobs(ctx, p, out.Fills)
			if err != nil {
				return nil, err
			}
			w.Failed = failed
			return w, nil
		}
		if time.Now().After(deadline) {
			return nil, &Error{Exit: ExitNetwork, Code: "wait_timeout",
				What: fmt.Sprintf("the AI jobs didn't finish within %s", a.timeout),
				Why:  fmt.Sprintf("%d of %d jobs finished (%d running, %d queued)", final, total, states["running"], states["queued"]),
				Fix:  "wait longer (--timeout), or check the jobs in Studio; they keep running on the server"}
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(a.pollInterval):
		}
	}
}

func (inv *invocation) failedJobs(ctx context.Context, p *project, fills []fillJSON) ([]failedJobJSON, error) {
	out := []failedJobJSON{}
	for _, f := range fills {
		for _, state := range []string{"failed", "dead"} {
			if f.JobStates[state] == 0 {
				continue
			}
			jobs, err := p.client.AIJobs(ctx, p.scope.Tenant, remote.JobFilter{Fill: f.ID, State: state})
			if err != nil {
				return nil, inv.m2Error(err, "can't list the failed jobs")
			}
			for _, j := range jobs {
				out = append(out, failedJobJSON{ID: j.Id, Key: j.MessageKey, Locale: j.Locale, State: string(j.State),
					FailureCode: derefStr(j.FailureCode), Error: derefStr(j.LastError)})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Locale != out[j].Locale {
			return out[i].Locale < out[j].Locale
		}
		return out[i].Key < out[j].Key
	})
	return out, nil
}

func printWait(pr *printer, w waitJSON, locales []string) {
	total := 0
	var parts []string
	for _, s := range []string{"succeeded", "skipped", "failed", "dead", "cancelled"} {
		if n := w.JobStates[s]; n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, s))
		}
		total += w.JobStates[s]
	}
	mark := pr.pass()
	if len(w.Failed) > 0 {
		mark = pr.fail()
	}
	pr.line("%s %s finished: %s", mark, plural(total, "job", "jobs"), orDefault(strings.Join(parts, ", "), "none"))
	for i, f := range w.Failed {
		if i == maxFindingsPerGroup {
			pr.line("  %s", pr.dim(fmt.Sprintf("… and %d more (--json lists all)", len(w.Failed)-i)))
			break
		}
		pr.line("  %s %s %s  %s: %s", pr.bad(f.State), f.Locale, f.Key, orDefault(f.FailureCode, "error"), f.Error)
	}
	if w.JobStates["succeeded"] > 0 {
		pr.line("  %s", pr.dim(fmt.Sprintf("`glossa review list --locale %s` shows the suggestions waiting for review", strings.Join(locales, ","))))
	}
}
