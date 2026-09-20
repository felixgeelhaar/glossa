package cli

import (
	"strings"
	"testing"

	"github.com/felixgeelhaar/glossa/platform/internal/cli/terminology"
)

// termbase adds the cart concept: en "cart" preferred, de "Warenkorb"
// preferred and "Einkaufswagen" forbidden.
func termbase(t *testing.T, w *workspace) conceptJSON {
	t.Helper()
	var out termsChangeJSON
	w.json(&out, "terms", "add", "cart", "--preferred", "de=Warenkorb", "--forbidden", "de=Einkaufswagen",
		"--definition", "Where items wait before checkout", "--domain", "shop").want(t, ExitOK)
	return out.Concept
}

func TestTermsAddListShow(t *testing.T) {
	_, w := seeded(t)
	var added termsChangeJSON
	w.json(&added, "terms", "add", "cart", "--preferred", "de=Warenkorb", "--forbidden", "de=Einkaufswagen",
		"--definition", "Where items wait", "--part-of-speech", "noun").want(t, ExitOK)
	c := added.Concept
	if added.Schema != "glossa.cli.terms.change/v1" || added.Action != "created" || c.ProjectID == nil || *c.ProjectID != "prj_1" ||
		len(c.Terms) != 3 || c.Terms[0].Locale != "en" || c.Terms[0].Status != "preferred" || c.Terms[2].Status != "forbidden" {
		t.Fatalf("add = %+v", added)
	}
	var tenant termsChangeJSON
	w.json(&tenant, "terms", "add", "--preferred", "en=checkout", "--tenant-wide").want(t, ExitOK)
	if tenant.Concept.ProjectID != nil {
		t.Errorf("tenant-wide concept = %+v", tenant.Concept)
	}

	var list termsListJSON
	w.json(&list, "terms", "list").want(t, ExitOK)
	if list.Schema != "glossa.cli.terms.list/v1" || len(list.Concepts) != 2 {
		t.Fatalf("list = %+v", list)
	}
	w.json(&list, "terms", "list", "--query", "waren").want(t, ExitOK)
	if len(list.Concepts) != 1 || list.Concepts[0].ID != c.ID {
		t.Errorf("list --query = %+v", list)
	}
	h := w.run("terms", "list")
	if !strings.Contains(h.stdout, "cart (en) · Warenkorb (de) · Einkaufswagen (de, forbidden)") || !strings.Contains(h.stdout, "tenant") {
		t.Errorf("human list:\n%s", h.stdout)
	}

	var show termsShowJSON
	w.json(&show, "terms", "show", "Warenkorb").want(t, ExitOK)
	if show.Schema != "glossa.cli.terms.show/v1" || show.Concept.ID != c.ID {
		t.Fatalf("show by term = %+v", show)
	}
	w.json(&show, "terms", "show", c.ID).want(t, ExitOK)
	if show.Concept.Definition != "Where items wait" {
		t.Errorf("show by ID = %+v", show)
	}
	if h := w.run("terms", "show", c.ID); !strings.Contains(h.stdout, "Einkaufswagen") || !strings.Contains(h.stdout, "forbidden") {
		t.Errorf("human show:\n%s", h.stdout)
	}
	var e errorDoc
	w.json(&e, "terms", "show", "basket").want(t, ExitNetwork)
	if e.Error.Code != "term_not_found" || e.Error.Fix == "" {
		t.Errorf("unknown term = %+v", e)
	}
	w.json(&e, "terms", "add", "cart", "--preferred", "en=Cart").want(t, ExitUsage)
	if e.Error.Code != "duplicate_term" {
		t.Errorf("duplicate = %+v", e)
	}
	w.run("terms", "add").want(t, ExitUsage)
	w.run("terms", "add", "--preferred", "de").want(t, ExitUsage)
	w.run("terms", "add", "x", "--locale", "de", "--locale", "en").want(t, ExitUsage)
	w.run("terms").want(t, ExitUsage)
	w.run("terms", "import").want(t, ExitUsage)
}

func TestTermsEditDeprecateForbid(t *testing.T) {
	_, w := seeded(t)
	c := termbase(t, w)

	var out termsChangeJSON
	w.json(&out, "terms", "edit", c.ID, "--admitted", "de=Korb", "--remove", "de=Einkaufswagen", "--note", "UI term").want(t, ExitOK)
	if out.Action != "updated" || out.Concept.Version != c.Version+1 || out.Concept.Note != "UI term" || out.Concept.Definition != c.Definition ||
		termsSummary(out.Concept.Terms) != "cart (en) · Warenkorb (de) · Korb (de, admitted)" {
		t.Fatalf("edit = %+v", out)
	}
	w.json(&out, "terms", "edit", c.ID, "--note", "UI term").want(t, ExitOK)
	if out.Action != "unchanged" {
		t.Errorf("same edit = %+v", out)
	}

	w.json(&out, "terms", "deprecate", "Korb").want(t, ExitOK)
	if out.Action != "deprecated" || out.Concept.Terms[2].Status != "deprecated" {
		t.Fatalf("deprecate = %+v", out)
	}
	// With --concept, forbid adds a term the concept lacks.
	w.json(&out, "terms", "forbid", "basket", "--locale", "en", "--concept", c.ID).want(t, ExitOK)
	if out.Action != "forbidden" || termsSummary(out.Concept.Terms) != "cart (en) · Warenkorb (de) · Korb (de, deprecated) · basket (en, forbidden)" {
		t.Fatalf("forbid = %+v", out)
	}
	if h := w.run("terms", "forbid", "Warenkorb"); !strings.Contains(h.stdout, "forbidden") {
		t.Errorf("human forbid:\n%s", h.stdout)
	}

	var e errorDoc
	w.json(&e, "terms", "deprecate", "nothing").want(t, ExitNetwork)
	if e.Error.Code != "term_not_found" {
		t.Errorf("unknown term = %+v", e)
	}
	w.run("terms", "forbid", "basket", "--concept", c.ID).want(t, ExitOK) // already a term: its locale is known
	w.run("terms", "forbid", "trolley", "--concept", c.ID).want(t, ExitUsage)
	w.run("terms", "edit", c.ID, "--remove", "de=nope").want(t, ExitUsage)
	w.run("terms", "edit", "not-an-id").want(t, ExitUsage)
	w.run("terms", "forbid", "x", "--concept", "nope").want(t, ExitUsage)
}

func TestTermsCheckFindsForbiddenTerms(t *testing.T) {
	srv, w := seeded(t)
	termbase(t, w)
	w.write("locales/de.json", `{"cart.checkout": "Zum Einkaufswagen", "cart.items": "{count, plural, one {# Artikel} other {# Artikel}}", "checkout.pay": "Zahle {amount, number}"}`)
	w.run("push", "--translations").want(t, ExitOK)
	w.write("locales/en.json", `{"cart": {"checkout": "Your cart", "items": "{count, plural, one {# item} other {# items}}"}, "checkout.pay": "Pay {amount, number}"}`)
	w.run("push").want(t, ExitOK)

	var out termsCheckJSON
	w.json(&out, "terms", "check").want(t, ExitCheckFailed)
	if out.Schema != "glossa.cli.terms.check/v1" || out.Passed || out.Errors != 1 || out.Checked != 5 || len(out.Locales) != 2 {
		t.Fatalf("check = %+v", out)
	}
	var codes []string
	for _, f := range out.Findings {
		codes = append(codes, f.Locale+" "+f.Key+" "+f.Code+" "+f.Severity)
	}
	if strings.Join(codes, ",") != "de cart.checkout term_forbidden error,de cart.checkout term_missing warning" {
		t.Errorf("findings = %v", codes)
	}
	if f := out.Findings[0]; f.Text != "Einkaufswagen" || strings.Join(f.Suggestions, ",") != "Warenkorb" {
		t.Errorf("forbidden = %+v", f)
	}
	// The server checks every translation: one page for the project, no
	// request per translation, no snapshot of the project.
	srv.mu.Lock()
	pages, checks := srv.kn.termPages, srv.kn.termChecks
	srv.mu.Unlock()
	if pages != 1 || checks != 0 || srv.countRequests("GET /v1/tenants/ten_1/projects/prj_1/translations") != 0 {
		t.Errorf("terms check sent %d pages, %d single checks", pages, checks)
	}

	h := w.run("terms", "check")
	h.want(t, ExitCheckFailed)
	for _, want := range []string{"✓ 5 translations checked against the termbase", "✗ de 1 error, 1 warning",
		`error cart.checkout  term_forbidden: "Einkaufswagen" is forbidden (use "Warenkorb")`, "✓ ja no findings", "Terminology check failed"} {
		if !strings.Contains(h.stdout, want) {
			t.Errorf("human check lacks %q:\n%s", want, h.stdout)
		}
	}

	// Only ja: nothing to find; --states narrows what is checked.
	w.json(&out, "terms", "check", "--locale", "ja").want(t, ExitOK)
	if !out.Passed || len(out.Locales) != 1 || out.Checked != 2 {
		t.Errorf("ja only = %+v", out)
	}
	w.json(&out, "terms", "check", "--states", "approved").want(t, ExitOK)
	if out.Checked != 0 {
		t.Errorf("approved only = %+v", out)
	}
	w.run("terms", "check", "--locale", "fr").want(t, ExitUsage)
	w.run("terms", "check", "--states", "done").want(t, ExitUsage)

	// glossa check --terminology adds the layer to structural QA.
	var chk checkJSON
	w.json(&chk, "check", "--terminology", "--require-complete=none").want(t, ExitCheckFailed)
	found := false
	for _, f := range chk.Findings {
		if f.Check == terminology.CheckName && f.Code == "term_forbidden" && f.Locale == "de" && f.Key == "cart.checkout" {
			found = true
		}
	}
	if !found || chk.Errors != 1 {
		t.Errorf("check --terminology = %+v", chk)
	}
	srv.mu.Lock()
	before := srv.kn.termPages
	srv.mu.Unlock()
	w.json(&chk, "check", "--require-complete=none").want(t, ExitOK)
	srv.mu.Lock()
	after := srv.kn.termPages
	srv.mu.Unlock()
	if after != before {
		t.Error("plain check asked the termbase")
	}
	w.run("check", "--terminology", "--offline").want(t, ExitUsage)
}
