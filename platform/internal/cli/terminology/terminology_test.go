package terminology

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/felixgeelhaar/glossa/platform/internal/cli/qa"
	"github.com/felixgeelhaar/glossa/platform/internal/cli/remote"
	"github.com/felixgeelhaar/glossa/platform/internal/cli/snapshot"
)

func snap(t *testing.T) *snapshot.Snapshot {
	t.Helper()
	msg := func(key, text string) snapshot.Message {
		m, args, inv := snapshot.Parse("mf1", text, "en")
		if inv != nil {
			t.Fatal(inv)
		}
		return snapshot.Message{Key: key, Text: text, Syntax: "mf1", Model: m, Arguments: args}
	}
	tr := func(key, locale, syntax, text, state string) snapshot.Translation {
		m, _, inv := snapshot.Parse(syntax, text, locale)
		if inv != nil {
			t.Fatal(inv)
		}
		return snapshot.Translation{Key: key, Locale: locale, Text: text, Syntax: syntax, Model: m, State: state}
	}
	return &snapshot.Snapshot{
		SourceLocale: "en",
		Locales:      []snapshot.Locale{{Code: "en", IsSource: true}, {Code: "de"}, {Code: "fr"}},
		Messages:     []snapshot.Message{msg("a", "Your cart"), msg("b", "Pay {amount, number}")},
		Translations: map[string]map[string]snapshot.Translation{
			"de": {"a": tr("a", "de", "mf1", "Dein Einkaufswagen", "needs_review"), "b": tr("b", "de", "mf2", "Zahle {$amount :number}", "approved")},
			"fr": {"a": tr("a", "fr", "mf1", "Panier", "rejected")},
		},
	}
}

func TestRunChecksSelectedTranslations(t *testing.T) {
	var reqs []remote.TerminologyRequest
	check := func(_ context.Context, req remote.TerminologyRequest) (remote.TerminologyCheck, error) {
		reqs = append(reqs, req)
		if req.Target == "Dein Einkaufswagen" {
			return remote.TerminologyCheck{Findings: []remote.TermFinding{{Code: "term_forbidden", Severity: "error", Side: "target",
				Text: "Einkaufswagen", Message: "forbidden", Suggestions: []string{"Warenkorb"}}}}, nil
		}
		return remote.TerminologyCheck{}, nil
	}
	r, err := Run(context.Background(), snap(t), Options{Project: "p", Concurrency: 1}, check)
	if err != nil {
		t.Fatal(err)
	}
	// fr's only translation is rejected: not checked.
	if r.Checked != 2 || r.Errors != 1 || len(r.Locales) != 2 || r.Locales[0].Errors != 1 || r.Locales[1].Checked != 0 {
		t.Fatalf("report = %+v", r)
	}
	// Different syntaxes are both sent as MF2.
	for _, req := range reqs {
		if req.Target == "Zahle {$amount :number}" && (string(*req.Syntax) != "mf2" || req.Source == "Pay {amount, number}") {
			t.Errorf("mixed syntax request = %+v", req)
		}
		if *req.ProjectId != "p" || req.SourceLocale != "en" {
			t.Errorf("request = %+v", req)
		}
	}
	qf := r.QA()
	if len(qf) != 1 || qf[0].Check != CheckName || qf[0].Severity != qa.Error || qf[0].Locale != "de" || qf[0].Key != "a" {
		t.Errorf("qa = %+v", qf)
	}
	only, _ := Run(context.Background(), snap(t), Options{Locales: []string{"fr"}, States: []string{"rejected"}}, check)
	if only.Checked != 1 || len(only.Locales) != 1 {
		t.Errorf("fr rejected = %+v", only)
	}
}

func TestRunStopsAtTheFirstError(t *testing.T) {
	var calls atomic.Int32
	boom := errors.New("boom")
	_, err := Run(context.Background(), snap(t), Options{}, func(context.Context, remote.TerminologyRequest) (remote.TerminologyCheck, error) {
		calls.Add(1)
		return remote.TerminologyCheck{}, boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
}
