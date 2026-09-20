package prompts_test

import (
	"encoding/json"
	"io/fs"
	"regexp"
	"strings"
	"testing"

	mf "github.com/felixgeelhaar/glossa/messageformat"

	"github.com/felixgeelhaar/glossa/platform/internal/intelligence/prompts"
)

func fullData() prompts.Data {
	return prompts.Data{
		SourceLocale: "en", TargetLocale: "pl",
		Source: ".input {$count :number}\n.match $count\none {{You have {$count} {#b}new{/b} file.}}\n* {{You have {$count} {#b}new{/b} files.}}",
		Arguments: []prompts.Argument{
			{Name: "count", Type: "number", Selector: "plural", Keys: []string{"one"}},
		},
		Markup:           []string{"b"},
		PluralCategories: []string{"one", "few", "many", "other"},
		Context: prompts.Context{
			Key: "files.count", Namespace: "files", Description: "Badge on the upload button", MaxLength: 40,
			Usages:     []string{"Upload dialog"},
			Neighbours: []prompts.Neighbour{{Key: "files.title", Source: "Files", Translation: "Pliki"}},
		},
		TM:    []prompts.TMMatch{{Score: 87, Source: "You have {1} new files.", Target: "Masz {1} nowych plików."}},
		Terms: []prompts.Term{{Source: "file", Definition: "an uploaded document", Use: []string{"plik"}, Avoid: []string{"dokument"}}},
		Style: prompts.Style{
			Formality: "informal", Pronoun: "ty", Tone: []string{"friendly"},
			Punctuation: []prompts.KeyValue{{Key: "quotes", Value: "„…”"}},
			Rules:       []prompts.Rule{{Rule: "Use sentence case", Rationale: "house style", Good: []string{"Nowy plik"}, Bad: []string{"Nowy Plik"}}},
		},
	}
}

func TestTranslateRender(t *testing.T) {
	tmpl := prompts.MustLoad(prompts.Translate, "v1")
	if tmpl.ID() != "translate/v1" {
		t.Fatalf("ID = %q", tmpl.ID())
	}
	out, err := tmpl.Render(fullData())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Translate this message from en to pl.",
		"<source>\n.input {$count :number}",
		"- $count: number, selects by plural (source keys: one, *)",
		"- markup: b",
		"- pl plural categories: one, few, many, other",
		"- description: Badge on the upload button",
		"- maximum length: 40 characters of visible text",
		"- nearby message files.title: Files → Pliki",
		`- "file" (an uploaded document); use: plik; avoid: dokument`,
		`- form of address: informal ("ty")`,
		"- quotes: „…”",
		"- Use sentence case (house style) Good: Nowy plik. Bad: Nowy Plik.",
		"- 87% match: You have {1} new files. → Masz {1} nowych plików.",
	} {
		if !strings.Contains(out.User, want) {
			t.Errorf("user prompt lacks %q:\n%s", want, out.User)
		}
	}
	if strings.Contains(out.System, "[[") || strings.Contains(out.User, "[[") {
		t.Error("unrendered template action")
	}
	if regexp.MustCompile(`\n\n\n`).MatchString(out.User) {
		t.Errorf("runs of blank lines:\n%q", out.User)
	}
	// The system prompt carries no per-message data, so it caches.
	other, err := tmpl.Render(prompts.Data{SourceLocale: "de", TargetLocale: "fr", Source: "Hallo"})
	if err != nil {
		t.Fatal(err)
	}
	if other.System != out.System {
		t.Error("system prompt depends on message data")
	}
	if strings.Contains(other.User, "<glossary>") || strings.Contains(other.User, "<structure>") {
		t.Errorf("empty sections rendered:\n%s", other.User)
	}
}

// The examples in the system prompt must themselves be valid MF2 with the
// structure the rules demand, or the model learns wrong syntax.
func TestTranslateSystemExamplesAreValidMF2(t *testing.T) {
	out, err := prompts.MustLoad(prompts.Translate, "v1").Render(prompts.Data{Source: "x"})
	if err != nil {
		t.Fatal(err)
	}
	answers := regexp.MustCompile(`(?m)^\{"message": .*\}$`).FindAllString(out.System, -1)
	if len(answers) != 2 {
		t.Fatalf("found %d example answers", len(answers))
	}
	src, err := mf.ParseMF2(".input {$count :number}\n.match $count\none {{You have {$count} new message.}}\n* {{You have {$count} new messages.}}")
	if err != nil {
		t.Fatal(err)
	}
	for i, a := range answers {
		var ans prompts.DraftAnswer
		if err := json.Unmarshal([]byte(a), &ans); err != nil {
			t.Fatalf("example %d is not JSON: %v", i, err)
		}
		msg, err := mf.ParseMF2(ans.Message)
		if err != nil {
			t.Fatalf("example %d is not MF2: %v", i, err)
		}
		if i == 0 {
			if f := mf.CheckCompat(src, msg, "pl"); len(f) != 0 {
				t.Errorf("Polish example findings: %+v", f)
			}
		}
	}
}

func TestRepairRender(t *testing.T) {
	d := fullData()
	d.Attempt, d.MaxAttempts = 1, 2
	d.Findings = []prompts.Finding{{Code: "missing-plural-category", Subject: "count", Message: "few falls back to *"}}
	out, err := prompts.MustLoad(prompts.Repair, "v1").Render(d)
	if err != nil {
		t.Fatal(err)
	}
	if out.System != "" {
		t.Error("the repair turn continues the translate conversation; it has no system part")
	}
	for _, want := range []string{"attempt 1 of 2", "- missing-plural-category (count): few falls back to *", "The pl plural categories are: one, few, many, other"} {
		if !strings.Contains(out.User, want) {
			t.Errorf("repair prompt lacks %q:\n%s", want, out.User)
		}
	}
}

func TestAssessRender(t *testing.T) {
	d := fullData()
	d.Translation = "Masz {$count} nowe pliki."
	out, err := prompts.MustLoad(prompts.Assess, "v1").Render(d)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Review this en → pl translation.", "<translation>\nMasz {$count} nowe pliki.\n</translation>", `use: plik`, "form of address: informal"} {
		if !strings.Contains(out.User, want) {
			t.Errorf("assess prompt lacks %q:\n%s", want, out.User)
		}
	}
}

func TestLoadUnknown(t *testing.T) {
	if _, err := prompts.Load(prompts.Translate, "v0"); err == nil {
		t.Error("want an error for an unknown version")
	}
}

func TestEveryEmbeddedTemplateLoads(t *testing.T) {
	for _, task := range []string{prompts.Translate, prompts.Repair, prompts.Assess} {
		entries, err := fs.Glob(prompts.Files(), task+"/*.tmpl")
		if err != nil || len(entries) == 0 {
			t.Fatalf("%s: no templates (%v)", task, err)
		}
		for _, e := range entries {
			version := strings.TrimSuffix(strings.TrimPrefix(e, task+"/"), ".tmpl")
			if _, err := prompts.Load(task, version); err != nil {
				t.Error(err)
			}
		}
	}
}

func TestSchemasAreJSON(t *testing.T) {
	for _, s := range []map[string]any{prompts.DraftSchema(), prompts.AssessSchema()} {
		if _, err := json.Marshal(s); err != nil {
			t.Error(err)
		}
	}
}
