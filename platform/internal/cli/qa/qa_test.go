package qa_test

import (
	"testing"

	mf "github.com/felixgeelhaar/glossa/messageformat"

	"github.com/felixgeelhaar/glossa/platform/internal/cli/qa"
	"github.com/felixgeelhaar/glossa/platform/internal/cli/snapshot"
	"github.com/felixgeelhaar/glossa/platform/internal/quality/domain"
)

func msg(key, text string) snapshot.Message {
	model, args, invalid := snapshot.Parse("mf1", text, "en")
	return snapshot.Message{Key: key, Text: text, Model: model, Arguments: args, Invalid: invalid}
}

func tr(key, locale, text string) snapshot.Translation {
	model, _, invalid := snapshot.Parse("mf1", text, locale)
	return snapshot.Translation{Key: key, Locale: locale, Text: text, Model: model, Invalid: invalid, State: "approved"}
}

func project() *snapshot.Snapshot {
	return &snapshot.Snapshot{
		Origin: "server", SourceLocale: "en",
		Locales: []snapshot.Locale{{Code: "en", IsSource: true}, {Code: "de"}, {Code: "ja"}},
		Messages: []snapshot.Message{
			msg("cart.items", "{count, plural, one {# item} other {# items}}"),
			msg("checkout.pay", "Pay {amount, number}"),
		},
		Translations: map[string]map[string]snapshot.Translation{
			"de": {
				"cart.items":   tr("cart.items", "de", "{count, plural, one {# Artikel} other {# Artikel}}"),
				"checkout.pay": tr("checkout.pay", "de", "Bezahlen"),
			},
			"ja": {"cart.items": tr("cart.items", "ja", "{count}個")},
		},
	}
}

func codes(fs []qa.Finding) map[string]qa.Severity {
	out := map[string]qa.Severity{}
	for _, f := range fs {
		out[f.Locale+" "+f.Key+" "+f.Code] = f.Severity
	}
	return out
}

func TestRunReportsCompatAndMissingTranslations(t *testing.T) {
	r := qa.Run(project(), qa.Policy{}, qa.Default()...)
	got := codes(r.Findings)
	for key, sev := range map[string]qa.Severity{
		"de checkout.pay " + string(mf.FindingMissingArgument): qa.Error,
		"ja checkout.pay " + qa.CodeMissingTranslation:         qa.Error,
	} {
		if got[key] != sev {
			t.Errorf("%s = %q, want %q (all: %v)", key, got[key], sev, got)
		}
	}
	if r.Passed {
		t.Error("check passed with errors")
	}
	ja := r.Locales[2]
	if ja.Code != "ja" || ja.Missing != 1 || ja.Translated != 1 || ja.Complete || !ja.Required {
		t.Errorf("ja report = %+v", ja)
	}
	if r.Locales[0].Code != "en" || !r.Locales[0].IsSource || r.Locales[0].Required {
		t.Errorf("source report = %+v", r.Locales[0])
	}
}

func TestPolicyRequireCompleteAndFailOn(t *testing.T) {
	s := project()
	s.Translations["de"]["checkout.pay"] = tr("checkout.pay", "de", "Bezahle {amount, number}")
	r := qa.Run(s, qa.Policy{RequireComplete: []string{"de"}}, qa.Default()...)
	if got := codes(r.Findings)["ja checkout.pay "+qa.CodeMissingTranslation]; got != qa.Warning {
		t.Fatalf("ja missing = %q, want a warning when only de is required", got)
	}
	if !r.Passed {
		t.Errorf("warnings failed the check: %+v", r.Findings)
	}
	if r = qa.Run(s, qa.Policy{RequireComplete: []string{"de"}, FailOn: qa.Warning}, qa.Default()...); r.Passed {
		t.Error("fail-on warning passed with warnings")
	}
	r = qa.Run(s, qa.Policy{RequireComplete: []string{"de", "fr"}}, qa.Default()...)
	if got := codes(r.Findings)["fr  "+qa.CodeMissingLocale]; got != qa.Error || r.Passed {
		t.Errorf("required fr not in the project = %q (passed %v)", got, r.Passed)
	}
}

func TestOutdatedAndInvalidAndUnknown(t *testing.T) {
	s := project()
	s.Messages = append(s.Messages, msg("broken", "{oops"))
	out := tr("cart.items", "de", "{count, plural, one {# Artikel} other {# Artikel}}")
	out.Outdated = true
	s.Translations["de"]["cart.items"] = out
	s.Translations["de"]["gone"] = tr("gone", "de", "Weg")
	s.Translations["ja"]["checkout.pay"] = tr("checkout.pay", "ja", "{amount")
	got := codes(qa.Run(s, qa.Policy{RequireComplete: []string{}}, qa.Default()...).Findings)
	for key, sev := range map[string]qa.Severity{
		"en broken " + qa.CodeInvalidMessage:           qa.Error,
		"de cart.items " + qa.CodeOutdatedTranslation:  qa.Warning,
		"de gone " + qa.CodeUnknownKey:                 qa.Warning,
		"ja checkout.pay " + qa.CodeInvalidTranslation: qa.Error,
	} {
		if got[key] != sev {
			t.Errorf("%s = %q, want %q", key, got[key], sev)
		}
	}
}

func TestServerWarningsAreKeptWhenTheKernelCantComputeThem(t *testing.T) {
	s := project()
	de := s.Translations["de"]["cart.items"]
	de.Warnings = []mf.Finding{{Code: "max-length-exceeded", Severity: mf.SeverityWarning, Message: "too long"}}
	s.Translations["de"]["cart.items"] = de
	if got := codes(qa.Run(s, qa.Policy{}, qa.Default()...).Findings)["de cart.items max-length-exceeded"]; got != qa.Warning {
		t.Errorf("max-length-exceeded = %q", got)
	}
}

// The QA moved to internal/quality in M4; the command's wire shape did
// not. A finding still names its layer under the name `glossa check`
// has printed since M1 — "arguments", not "parity" — and still carries
// the kernel's subject and the local file it came from.
func TestTheWireShapeIsUnchanged(t *testing.T) {
	s := project()
	names := map[string]bool{}
	var compat qa.Finding
	for _, f := range qa.Run(s, qa.Policy{}, qa.Default()...).Findings {
		names[f.Check] = true
		if f.Code == string(mf.FindingMissingArgument) {
			compat = f
		}
	}
	for _, want := range []string{"arguments", "completeness"} {
		if !names[want] {
			t.Errorf("no finding named the %q check: %v", want, names)
		}
	}
	if names["parity"] {
		t.Error("the command printed the layer's new name; wave 3 renames it, not wave 1")
	}
	if compat.Subject != "amount" || compat.Message == "" || compat.Locale != "de" || compat.Key != "checkout.pay" {
		t.Errorf("the kernel's finding lost something on the way to the command: %+v", compat)
	}
}

// Terminology findings reach the command as they always did, through
// the precomputed layer.
func TestPrecomputedFindingsKeepTheirLayer(t *testing.T) {
	f := domain.New(domain.Finding{
		Layer: domain.LayerTerminology, Code: "term_forbidden", Severity: qa.Error,
		Locus: domain.Locus{Key: "cart.checkout", Locale: "de"}, Message: "forbidden term", Subject: "Einkaufswagen",
	})
	r := qa.Run(project(), qa.Policy{RequireComplete: []string{}},
		qa.Precomputed(domain.LayerTerminology, []domain.Finding{f}))
	if len(r.Findings) != 1 {
		t.Fatalf("findings = %+v", r.Findings)
	}
	got := r.Findings[0]
	if got.Check != "terminology" || got.Code != "term_forbidden" || got.Severity != qa.Error ||
		got.Locale != "de" || got.Key != "cart.checkout" || got.Subject != "Einkaufswagen" {
		t.Errorf("finding = %+v", got)
	}
	if r.Passed {
		t.Error("an error finding passed the check")
	}
}
