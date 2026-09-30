package domain_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/felixgeelhaar/glossa/platform/internal/kernel/checkpolicy"
	"github.com/felixgeelhaar/glossa/platform/internal/release/domain"
)

// built is a release shipping messages source messages, with the
// locales it ships and how many of them each carries.
func built(messages int, locales map[string]int) domain.Built {
	b := domain.Built{Stats: domain.Stats{Messages: messages, Locales: map[string]domain.LocaleStats{}}}
	for code, n := range locales {
		b.Stats.Locales[code] = domain.LocaleStats{Messages: n}
		b.Content.Locales = append(b.Content.Locales, domain.Locale{Code: code, Direction: "ltr"})
	}
	return b
}

// approvedOnly is what production and staging ship.
var approvedOnly = domain.Policy{States: []string{domain.StateApproved}}

// everything is what a preview or a branch ships.
var everything = domain.Policy{States: []string{domain.StateDraft, domain.StateNeedsReview, domain.StateApproved}}

func TestPolicyGateUnnamedEnvironmentIsNotGated(t *testing.T) {
	// The document requires every locale (its zero value), and says
	// nothing about staging. Publishing to staging is what it always
	// was: a project that never wrote a policy must not discover that
	// every environment silently became strict.
	g := domain.NewPolicyGate(checkpolicy.Policy{}, "staging")
	if g.Bound {
		t.Fatalf("staging is gated by a policy that does not name it: %+v", g)
	}
	if err := g.Check(everything, built(10, map[string]int{"en": 10, "de": 3})); err != nil {
		t.Fatalf("Check = %v, want nil", err)
	}
}

func TestPolicyGateRefusesAnIncompleteLocale(t *testing.T) {
	doc := checkpolicy.Policy{Environments: map[string]checkpolicy.Environment{
		"production": {RequireComplete: checkpolicy.RequiredLocales("de", "en")},
	}}
	g := domain.NewPolicyGate(doc, "production")
	err := g.Check(approvedOnly, built(10, map[string]int{"en": 10, "de": 7, "fr": 1}))
	if !errors.Is(err, domain.ErrPolicyNotMet) {
		t.Fatalf("Check = %v, want ErrPolicyNotMet", err)
	}
	var notMet *domain.PolicyNotMetError
	if !errors.As(err, &notMet) {
		t.Fatalf("Check = %v, want a *PolicyNotMetError", err)
	}
	if len(notMet.Unmet) != 1 || notMet.Unmet[0].Locale != "de" {
		t.Fatalf("unmet = %+v, want de alone (fr is not required)", notMet.Unmet)
	}
	if !strings.Contains(err.Error(), "de") {
		t.Errorf("the refusal does not name the locale: %s", err)
	}
	// Complete in both required locales publishes, however far behind
	// the locales the environment does not require are.
	if err := g.Check(approvedOnly, built(10, map[string]int{"en": 10, "de": 10, "fr": 0})); err != nil {
		t.Fatalf("Check = %v, want nil", err)
	}
}

func TestPolicyGateRequireCompleteNullIsEveryLocale(t *testing.T) {
	doc := checkpolicy.Policy{
		RequireComplete: []string{"de"},
		Environments:    map[string]checkpolicy.Environment{"production": {RequireComplete: checkpolicy.AllLocales()}},
	}
	g := domain.NewPolicyGate(doc, "production")
	if g.RequireComplete != nil {
		t.Fatalf("RequireComplete = %v, want nil (every locale)", g.RequireComplete)
	}
	err := g.Check(approvedOnly, built(4, map[string]int{"en": 4, "de": 4, "fr": 3}))
	if !errors.Is(err, domain.ErrPolicyNotMet) {
		t.Fatalf("Check = %v, want ErrPolicyNotMet: null means every locale, fr among them", err)
	}
}

func TestPolicyGateInheritsTheDocumentsRequireComplete(t *testing.T) {
	// The block names a review state and no require_complete. "I did not
	// say" is not "every locale": it inherits the document's, which is
	// de alone. Getting this wrong makes production quietly stricter
	// than anyone wrote — the same property wave 3 pins on the CLI side.
	doc := checkpolicy.Policy{
		RequireComplete: []string{"de"},
		Environments:    map[string]checkpolicy.Environment{"production": {RequireReview: checkpolicy.ReviewApproved}},
	}
	g := domain.NewPolicyGate(doc, "production")
	if len(g.RequireComplete) != 1 || g.RequireComplete[0] != "de" {
		t.Fatalf("RequireComplete = %v, want [de] inherited from the document", g.RequireComplete)
	}
	// fr is nowhere near complete and is not required anywhere.
	if err := g.Check(approvedOnly, built(4, map[string]int{"en": 4, "de": 4, "fr": 0})); err != nil {
		t.Fatalf("Check = %v, want nil: only de is required", err)
	}
}

func TestPolicyGateInheritsEveryLocaleWhenTheDocumentSaysSo(t *testing.T) {
	// The document's nil require_complete is "every locale", and a block
	// that names none inherits that too.
	doc := checkpolicy.Policy{Environments: map[string]checkpolicy.Environment{
		"production": {RequireReview: checkpolicy.ReviewApproved},
	}}
	g := domain.NewPolicyGate(doc, "production")
	if g.RequireComplete != nil {
		t.Fatalf("RequireComplete = %v, want nil (every locale)", g.RequireComplete)
	}
	if err := g.Check(approvedOnly, built(4, map[string]int{"en": 4, "fr": 3})); !errors.Is(err, domain.ErrPolicyNotMet) {
		t.Fatalf("Check = %v, want ErrPolicyNotMet", err)
	}
}

func TestPolicyGateEmptyRequireCompleteRequiresNothing(t *testing.T) {
	doc := checkpolicy.Policy{Environments: map[string]checkpolicy.Environment{
		"development": {RequireComplete: checkpolicy.RequiredLocales()},
	}}
	g := domain.NewPolicyGate(doc, "development")
	if !g.Bound {
		t.Fatal("the policy names development, so it is gated")
	}
	if err := g.Check(everything, built(9, map[string]int{"en": 9, "de": 0})); err != nil {
		t.Fatalf("Check = %v, want nil: an empty list requires no locale", err)
	}
}

func TestPolicyGateEnforcesRequireReview(t *testing.T) {
	doc := checkpolicy.Policy{Environments: map[string]checkpolicy.Environment{
		"production": {RequireComplete: checkpolicy.RequiredLocales(), RequireReview: checkpolicy.ReviewApproved},
	}}
	g := domain.NewPolicyGate(doc, "production")
	complete := built(3, map[string]int{"en": 3, "de": 3})
	// An environment shipping drafts has not reached approved, however
	// complete it is.
	err := g.Check(everything, complete)
	if !errors.Is(err, domain.ErrPolicyNotMet) {
		t.Fatalf("Check = %v, want ErrPolicyNotMet", err)
	}
	if !strings.Contains(err.Error(), checkpolicy.ReviewApproved) {
		t.Errorf("the refusal does not name the review state: %s", err)
	}
	if err := g.Check(approvedOnly, complete); err != nil {
		t.Fatalf("Check = %v, want nil: approved-only text has reached approved", err)
	}
}

func TestPolicyGateRequiresALocaleTheReleaseDoesNotShip(t *testing.T) {
	doc := checkpolicy.Policy{Environments: map[string]checkpolicy.Environment{
		"production": {RequireComplete: checkpolicy.RequiredLocales("ja")},
	}}
	g := domain.NewPolicyGate(doc, "production")
	if err := g.Check(approvedOnly, built(2, map[string]int{"en": 2})); !errors.Is(err, domain.ErrPolicyNotMet) {
		t.Fatalf("Check = %v, want ErrPolicyNotMet", err)
	}
}

func TestPolicyReached(t *testing.T) {
	cases := []struct {
		name  string
		p     domain.Policy
		state string
		want  bool
	}{
		{"approved only", approvedOnly, domain.StateApproved, true},
		{"everything", everything, domain.StateApproved, false},
		{"needs_review and approved", domain.Policy{States: []string{domain.StateNeedsReview, domain.StateApproved}}, domain.StateApproved, false},
		{"everything reaches draft", everything, domain.StateDraft, true},
		{"unknown state", approvedOnly, "published", false},
	}
	for _, tc := range cases {
		if got := tc.p.Reached(tc.state); got != tc.want {
			t.Errorf("%s: Reached(%q) = %v", tc.name, tc.state, got)
		}
	}
}

func TestNewOverride(t *testing.T) {
	o, err := domain.NewOverride("  the launch ships before the German copy  ")
	if err != nil {
		t.Fatalf("NewOverride = %v", err)
	}
	if !o.Forced || o.Reason != "the launch ships before the German copy" {
		t.Fatalf("override = %+v", o)
	}
	for _, blank := range []string{"", "   ", "\t\n"} {
		if _, err := domain.NewOverride(blank); !errors.Is(err, domain.ErrForceNeedsReason) {
			t.Errorf("NewOverride(%q) = %v, want ErrForceNeedsReason", blank, err)
		}
	}
	long := strings.Repeat("ü", domain.MaxForceReasonLen+1)
	if _, err := domain.NewOverride(long); !errors.Is(err, domain.ErrInvalidForceReason) {
		t.Errorf("NewOverride(long) = %v, want ErrInvalidForceReason", err)
	}
}
