package app_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/felixgeelhaar/glossa/platform/internal/intelligence/adapters/memory"
	"github.com/felixgeelhaar/glossa/platform/internal/intelligence/app"
	"github.com/felixgeelhaar/glossa/platform/internal/intelligence/domain"
)

// scriptedProvider answers each task from a queue of answers.
type scriptedProvider struct {
	mu      sync.Mutex
	answers map[domain.Task][]string
	errs    map[domain.Task]error
	calls   []domain.CompletionRequest
}

func (s *scriptedProvider) Name() string { return "anthropic" }

func (s *scriptedProvider) Complete(_ context.Context, req domain.CompletionRequest) (domain.Completion, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, req)
	if err := s.errs[req.Task]; err != nil {
		return domain.Completion{}, err
	}
	q := s.answers[req.Task]
	if len(q) == 0 {
		return domain.Completion{}, errors.New("script exhausted for " + string(req.Task))
	}
	s.answers[req.Task] = q[1:]
	return domain.Completion{Provider: "anthropic", Model: req.Model, Text: q[0], Stop: domain.StopEnd, Usage: domain.Usage{InputTokens: 1000, OutputTokens: 100}}, nil
}

func (s *scriptedProvider) tasks() []domain.Task {
	var out []domain.Task
	for _, c := range s.calls {
		out = append(out, c.Task)
	}
	return out
}

func draft(msg string) string {
	raw, _ := json.Marshal(map[string]string{"message": msg, "notes": ""})
	return string(raw)
}

func assessment(score float64, formality bool) string {
	raw, _ := json.Marshal(map[string]any{"score": score, "formality_ok": formality, "issues": []string{}})
	return string(raw)
}

type fixture struct {
	provider  *scriptedProvider
	knowledge *memory.Knowledge
	budget    *memory.Budget
	tr        *app.Translator
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	p := &scriptedProvider{answers: map[domain.Task][]string{}, errs: map[domain.Task]error{}}
	k := memory.NewKnowledge("t1")
	k.AddConcepts(memory.Concept{ID: "c-file", Terms: []domain.Term{
		{ID: "t-file", Text: "file", Locale: "en", Status: domain.TermPreferred},
		{ID: "t-datei", Text: "Datei", Locale: "de", Status: domain.TermPreferred},
		{ID: "t-plik", Text: "plik", Locale: "pl", Status: domain.TermPreferred},
		{ID: "t-file-de", Text: "File", Locale: "de", Status: domain.TermForbidden},
	}})
	k.SetStyle("de", domain.StyleGuide{Version: "style-7", Formality: domain.FormalityInformal, Pronoun: "du"})
	max := 40
	k.SetContext(domain.MessageContext{MessageID: "m1", Key: "files.count", Description: "Upload badge", MaxLength: &max})
	budget := memory.NewBudget(map[string]domain.MicroUSD{"t1": 10_000_000})
	policy := domain.RoutingPolicy{Rules: []domain.RoutingRule{
		{Task: domain.TaskTranslate, Routes: []domain.Route{{Provider: "anthropic", Model: "claude-sonnet-5", MaxTokens: 8000, Effort: "medium"}}},
		{Task: domain.TaskAssess, Routes: []domain.Route{{Provider: "anthropic", Model: "claude-haiku-4-5-20251001", MaxTokens: 1024}}},
	}}
	router := app.NewRouter(map[string]domain.Provider{"anthropic": p}, policy, app.DefaultPrices(), budget)
	tr, err := app.NewTranslator(app.Config{Router: router, Knowledge: k})
	if err != nil {
		t.Fatal(err)
	}
	return &fixture{provider: p, knowledge: k, budget: budget, tr: tr}
}

const pluralSource = ".input {$count :number}\n.match $count\none {{You have {$count} file.}}\n* {{You have {$count} files.}}"

func request(target, source string) domain.TranslationRequest {
	return domain.TranslationRequest{
		Scope: domain.Scope{TenantID: "t1", ProjectID: "p1"}, MessageID: "m1", Key: "files.count",
		SourceLocale: "en", TargetLocale: target, Source: source, ProviderConsent: true,
	}
}

const deTarget = ".input {$count :number}\n.match $count\none {{Du hast {$count} Datei.}}\n* {{Du hast {$count} Dateien.}}"

func TestAIDraftPassesFirstTime(t *testing.T) {
	f := newFixture(t)
	f.provider.answers[domain.TaskTranslate] = []string{draft(deTarget)}
	f.provider.answers[domain.TaskAssess] = []string{assessment(0.9, true)}

	res, err := f.tr.Translate(context.Background(), request("de", pluralSource))
	if err != nil {
		t.Fatal(err)
	}
	s := res.Suggestion
	if s.Provenance.Origin != domain.OriginAI || s.Provenance.Repairs != 0 || s.Provenance.Model != "claude-sonnet-5" ||
		s.Provenance.PromptVersion != "translate/v1" || s.Provenance.StyleVersion != "style-7" {
		t.Errorf("provenance = %+v", s.Provenance)
	}
	if !strings.Contains(strings.Join(s.Provenance.TermIDs, ","), "t-datei") {
		t.Errorf("term ids = %v", s.Provenance.TermIDs)
	}
	if !strings.HasPrefix(s.Message, ".input {$count :number}") || len(s.TermFindings) != 0 {
		t.Errorf("message = %q, term findings %+v", s.Message, s.TermFindings)
	}
	if s.Confidence.Score != 0.81 || s.Action != domain.ActionApproveRecommended {
		t.Errorf("confidence = %+v, action %s", s.Confidence, s.Action)
	}
	if len(s.Calls) != 2 || s.Calls[1].Task != domain.TaskAssess || s.Cost == 0 || s.Usage.InputTokens != 2000 {
		t.Errorf("calls = %+v, cost %v, usage %+v", s.Calls, s.Cost, s.Usage)
	}
	if got := f.provider.tasks(); len(got) != 2 {
		t.Errorf("model calls = %v", got)
	}
	// The prompt holds the knowledge the tools returned.
	user := f.provider.calls[0].Messages[0].Text
	for _, want := range []string{"de plural categories: one, other", `use: Datei; avoid: File`, `form of address: informal ("du")`, "maximum length: 40"} {
		if !strings.Contains(user, want) {
			t.Errorf("prompt lacks %q:\n%s", want, user)
		}
	}
	if !f.provider.calls[0].System[0].Cacheable || f.provider.calls[0].Output == nil {
		t.Error("the system prompt is cacheable and the answer schema-constrained")
	}
	// Privacy accounting: every provider call is disclosed with what it saw.
	if len(res.Disclosures) != 2 || res.Disclosures[0].MessageID != "m1" || res.Disclosures[0].Sent[0].Text != user {
		t.Errorf("disclosures = %+v", res.Disclosures)
	}
	tools := map[string]int{}
	for _, a := range res.Audit {
		tools[a.Tool]++
	}
	for _, name := range []string{app.ToolTMLookup, app.ToolTermLookup, app.ToolStyleRules, app.ToolMessageContext, app.ToolDraft, app.ToolValidate, app.ToolAssess} {
		if tools[name] == 0 {
			t.Errorf("audit lacks %s: %v", name, tools)
		}
	}
	if f.budget.Spent("t1") != s.Cost {
		t.Errorf("budget recorded %v, suggestion cost %v", f.budget.Spent("t1"), s.Cost)
	}
}

func TestTargetPluralCategoriesAndRepair(t *testing.T) {
	f := newFixture(t)
	broken := ".input {$count :number}\n.match $count\none {{Masz {$n} plik.}}\nfew {{Masz {$count} pliki.}}\nmany {{Masz {$count} plików.}}\n* {{Masz {$count} pliku.}}"
	fixed := ".input {$count :number}\n.match $count\none {{Masz {$count} plik.}}\nfew {{Masz {$count} pliki.}}\nmany {{Masz {$count} plików.}}\n* {{Masz {$count} pliku.}}"
	f.provider.answers[domain.TaskTranslate] = []string{draft(broken), draft(fixed)}
	f.provider.answers[domain.TaskAssess] = []string{assessment(0.8, true)}

	res, err := f.tr.Translate(context.Background(), request("pl", pluralSource))
	if err != nil {
		t.Fatal(err)
	}
	if res.Suggestion.Provenance.Repairs != 1 || res.Suggestion.Provenance.PromptVersion != "translate/v1+repair/v1" {
		t.Errorf("provenance = %+v", res.Suggestion.Provenance)
	}
	first := f.provider.calls[0].Messages[0].Text
	if !strings.Contains(first, "pl plural categories: one, few, many, other") {
		t.Errorf("the prompt must list the target's categories:\n%s", first)
	}
	repair := f.provider.calls[1]
	if len(repair.Messages) != 3 || repair.Messages[1].Role != domain.RoleAssistant || repair.Messages[1].Text != draft(broken) {
		t.Fatalf("the repair continues the conversation: %+v", repair.Messages)
	}
	if !strings.Contains(repair.Messages[2].Text, "extra-argument (n)") || !strings.Contains(repair.Messages[2].Text, "attempt 1 of 2") {
		t.Errorf("repair turn:\n%s", repair.Messages[2].Text)
	}
	if c := res.Suggestion.Confidence; c.Score >= 0.81 || !c.Has(domain.FactorRepairs) {
		t.Errorf("a repair lowers confidence: %+v", c)
	}
}

func TestInvalidOutputAfterTwoRepairs(t *testing.T) {
	f := newFixture(t)
	bad := draft("Du hast Dateien.") // drops $count and the selector
	f.provider.answers[domain.TaskTranslate] = []string{bad, "not json at all", bad}

	res, err := f.tr.Translate(context.Background(), request("de", pluralSource))
	if !errors.Is(err, domain.ErrInvalidOutput) {
		t.Fatalf("err = %v", err)
	}
	if got := f.provider.tasks(); len(got) != 3 {
		t.Errorf("calls = %v, want draft + 2 repairs and no assessment", got)
	}
	if len(res.Findings) == 0 || res.Suggestion.Message != "" {
		t.Errorf("result = %+v", res)
	}
	// The unparsable answer was sent back as an answer-format finding.
	if !strings.Contains(f.provider.calls[2].Messages[4].Text, "answer-format") {
		t.Errorf("second repair turn:\n%s", f.provider.calls[2].Messages[4].Text)
	}
	if len(res.Disclosures) != 3 {
		t.Errorf("every call is disclosed, even of a failed job: %d", len(res.Disclosures))
	}
}

func TestExactTMReuseWithoutModelCall(t *testing.T) {
	f := newFixture(t)
	f.knowledge.AddUnits(memory.TMUnit{ID: "u9", Pair: domain.LocalePair{Source: "en", Target: "de"}, Key: "files.count", Source: pluralSource, Target: deTarget})
	req := request("de", pluralSource)
	req.ProviderConsent = false // TM reuse needs no provider

	res, err := f.tr.Translate(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	s := res.Suggestion
	if s.Provenance.Origin != domain.OriginTranslationMemory || len(s.Provenance.TMUnitIDs) != 1 || s.Provenance.TMUnitIDs[0] != "u9" || s.Provenance.Model != "" {
		t.Errorf("provenance = %+v", s.Provenance)
	}
	if len(f.provider.calls) != 0 || len(res.Disclosures) != 0 || s.Cost != 0 {
		t.Errorf("TM reuse made model calls: %v", f.provider.tasks())
	}
	if s.Confidence.Score != 0.99 || s.Action != domain.ActionApproveRecommended {
		t.Errorf("confidence = %+v, %s", s.Confidence, s.Action)
	}
}

func TestExactTMWithTermFindingIsNotReused(t *testing.T) {
	f := newFixture(t)
	withForbidden := ".input {$count :number}\n.match $count\none {{Du hast {$count} File.}}\n* {{Du hast {$count} Files.}}"
	f.knowledge.AddUnits(memory.TMUnit{ID: "u9", Pair: domain.LocalePair{Source: "en", Target: "de"}, Source: pluralSource, Target: withForbidden})
	f.provider.answers[domain.TaskTranslate] = []string{draft(deTarget)}
	f.provider.answers[domain.TaskAssess] = []string{assessment(0.9, true)}

	res, err := f.tr.Translate(context.Background(), request("de", pluralSource))
	if err != nil {
		t.Fatal(err)
	}
	if res.Suggestion.Provenance.Origin != domain.OriginAI {
		t.Errorf("origin = %s", res.Suggestion.Provenance.Origin)
	}
	if !strings.Contains(f.provider.calls[0].Messages[0].Text, "100% match") {
		t.Error("the TM match is still shown to the model")
	}
}

func TestGuards(t *testing.T) {
	t.Run("no provider consent", func(t *testing.T) {
		f := newFixture(t)
		req := request("de", pluralSource)
		req.ProviderConsent = false
		_, err := f.tr.Translate(context.Background(), req)
		if !errors.Is(err, domain.ErrProviderConsent) || len(f.provider.calls) != 0 {
			t.Errorf("err = %v, calls %d", err, len(f.provider.calls))
		}
	})
	t.Run("sensitive namespace", func(t *testing.T) {
		f := newFixture(t)
		req := request("de", pluralSource)
		req.Tags = []string{domain.TagSensitive}
		_, err := f.tr.Translate(context.Background(), req)
		if !errors.Is(err, domain.ErrSensitive) || len(f.provider.calls) != 0 {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("invalid source", func(t *testing.T) {
		f := newFixture(t)
		if _, err := f.tr.Translate(context.Background(), request("de", "Hello {$name")); !errors.Is(err, domain.ErrUnparsable) {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("invalid request", func(t *testing.T) {
		f := newFixture(t)
		if _, err := f.tr.Translate(context.Background(), domain.TranslationRequest{}); err == nil {
			t.Error("want error")
		}
	})
}

func TestProviderFailures(t *testing.T) {
	t.Run("outage stops the job, transiently, with disclosures", func(t *testing.T) {
		f := newFixture(t)
		f.provider.errs[domain.TaskTranslate] = &domain.ProviderError{Provider: "anthropic", Kind: domain.KindUnavailable, Status: 529}
		res, err := f.tr.Translate(context.Background(), request("de", pluralSource))
		if !errors.Is(err, app.ErrAllRoutesFailed) || !app.Transient(err) {
			t.Fatalf("err = %v", err)
		}
		if len(res.Disclosures) != 1 {
			t.Errorf("the provider saw the text: %+v", res.Disclosures)
		}
	})
	t.Run("budget exceeded", func(t *testing.T) {
		f := newFixture(t)
		req := request("de", pluralSource)
		req.Scope.TenantID = "t1"
		f.budget = memory.NewBudget(nil)
		policy := domain.RoutingPolicy{Rules: []domain.RoutingRule{{Task: domain.TaskTranslate, Routes: []domain.Route{{Provider: "anthropic", Model: "claude-sonnet-5", MaxTokens: 100}}}}}
		tr, _ := app.NewTranslator(app.Config{Router: app.NewRouter(map[string]domain.Provider{"anthropic": f.provider}, policy, app.DefaultPrices(), f.budget), Knowledge: f.knowledge})
		_, err := tr.Translate(context.Background(), req)
		if !errors.Is(err, domain.ErrBudgetExceeded) || len(f.provider.calls) != 0 {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("a failed assessment is skipped", func(t *testing.T) {
		f := newFixture(t)
		f.provider.answers[domain.TaskTranslate] = []string{draft(deTarget)}
		f.provider.errs[domain.TaskAssess] = &domain.ProviderError{Provider: "anthropic", Kind: domain.KindRateLimited, Status: 429}
		res, err := f.tr.Translate(context.Background(), request("de", pluralSource))
		if err != nil {
			t.Fatal(err)
		}
		if res.Suggestion.Confidence.Score != 0.75 || res.Suggestion.Confidence.Has(domain.FactorSelfAssessment) {
			t.Errorf("confidence without assessment = %+v", res.Suggestion.Confidence)
		}
	})
}

func TestForbiddenTermRequiresReview(t *testing.T) {
	f := newFixture(t)
	f.provider.answers[domain.TaskTranslate] = []string{draft(".input {$count :number}\n.match $count\none {{Du hast {$count} File.}}\n* {{Du hast {$count} Files.}}")}
	f.provider.answers[domain.TaskAssess] = []string{assessment(0.95, true)}
	res, err := f.tr.Translate(context.Background(), request("de", pluralSource))
	if err != nil {
		t.Fatal(err)
	}
	s := res.Suggestion
	if s.Action != domain.ActionReviewRequired || len(s.TermFindings) != 2 {
		t.Errorf("action %s, term findings %+v", s.Action, s.TermFindings)
	}
}
