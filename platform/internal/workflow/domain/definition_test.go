package domain_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/felixgeelhaar/glossa/platform/internal/workflow/defaults"
	"github.com/felixgeelhaar/glossa/platform/internal/workflow/domain"
)

func read(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func compile(t *testing.T, doc []byte) *domain.Definition {
	t.Helper()
	d, err := domain.Compile(doc)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	return d
}

// TestTheRFCsExampleLoadsAndRuns is RFC 0006 §2.3's example, verbatim:
// it compiles through statekit.FromJSON, and stepping it does what the
// document says — assign the vendor, then ask two reviewers, then
// approve only once two people other than the author have.
func TestTheRFCsExampleLoadsAndRuns(t *testing.T) {
	d := compile(t, read(t, "testdata/definitions/vendor-then-four-eyes.json"))
	if d.Name != "vendor-then-four-eyes" || d.Subject != domain.SubjectTranslation || d.Schema != domain.SchemaV1 {
		t.Fatalf("definition = %s %s %s", d.Name, d.Subject, d.Schema)
	}
	if got := d.Guards["two_approvals"]; got.Use != "approvals_at_least" ||
		got.Params != (domain.ApprovalsAtLeast{N: 2, DistinctFromAuthor: true}) {
		t.Errorf("two_approvals = %+v", got)
	}

	subject := domain.Subject{Kind: domain.SubjectTranslation, Locale: "de", Author: "person:author"}
	state, step := d.Start(domain.Step{Subject: subject})
	if state != "translating" || len(step.Effects) != 1 || step.Effects[0].Use != "assign" {
		t.Fatalf("start: %s %+v", state, step.Effects)
	}
	if a := step.Effects[0].Params.(domain.Assign); a.To.Vendor != "lingua-gmbh" || a.Due.Std() != 72*time.Hour {
		t.Errorf("assign = %+v", a)
	}

	next := func(state string, e domain.EventName, s domain.Subject) (string, domain.Step, bool) {
		t.Helper()
		to, out, ok, err := d.Advance(state, domain.Step{Subject: s, Trigger: domain.Trigger{Event: e, Actor: "person:x"}})
		if err != nil {
			t.Fatal(err)
		}
		return to, out, ok
	}

	state, step, ok := next("translating", domain.EventAssignmentCompleted, subject)
	if !ok || state != "reviewing" || len(step.Effects) != 1 || step.Effects[0].Name != "ask_two_reviewers" {
		t.Fatalf("assignment.completed: %v %s %+v", ok, state, step.Effects)
	}

	// The author approving their own text, and one reviewer, are not two.
	subject.Approvers = []string{"person:author", "person:r1"}
	if to, _, ok := next("reviewing", domain.EventApprovalGranted, subject); ok || to != "reviewing" {
		t.Fatalf("one distinct approval moved the instance to %s", to)
	}
	subject.Approvers = append(subject.Approvers, "person:r2")
	state, step, ok = next("reviewing", domain.EventApprovalGranted, subject)
	if !ok || state != "done" || !d.Final(state) {
		t.Fatalf("two approvals: %v %s", ok, state)
	}
	if len(step.Effects) != 1 || step.Effects[0].Params != (domain.SetReviewState{State: "approved"}) {
		t.Errorf("effects = %+v, want set_review_state approved", step.Effects)
	}

	// An event the state doesn't react to is ignored, not an error.
	if to, _, ok := next("reviewing", domain.EventTranslationOutdated, subject); ok || to != "reviewing" {
		t.Errorf("an unrelated event moved the instance to %s", to)
	}
	// A state the version does not have is an error: rebase refuses it.
	if _, _, _, err := d.Advance("legal_review", domain.Step{Trigger: domain.Trigger{Event: domain.EventApprovalGranted}}); err == nil {
		t.Error("advancing from a state the chart lacks succeeded")
	}
}

// TestTheDefaultIsADocument holds §2.1 rule 2 from the other side: the
// default review workflow loads through the same Compile as any
// tenant's, with nothing compiled in, and waits for a reviewer's
// decision without making one.
func TestTheDefaultIsADocument(t *testing.T) {
	d := compile(t, defaults.Review())
	if d.Name != "review" || d.Subject != domain.SubjectTranslation {
		t.Fatalf("default = %s %s", d.Name, d.Subject)
	}
	if len(d.Actions) != 0 || len(d.Guards) != 0 {
		t.Errorf("the default binds guards %v and actions %v; it must only wait for Localization's review", d.Guards, d.Actions)
	}
	state, step := d.Start(domain.Step{})
	if state != "awaiting_review" || len(step.Effects) != 0 {
		t.Fatalf("start: %s %+v", state, step.Effects)
	}
	to, _, ok, err := d.Advance(state, domain.Step{Trigger: domain.Trigger{Event: domain.EventTranslationReviewed}})
	if err != nil || !ok || !d.Final(to) {
		t.Fatalf("translation.reviewed: %s %v %v", to, ok, err)
	}
}

// TestRefusedDefinitions is lint-on-save: every fixture under
// testdata/definitions/refused is refused, with the rule its file name
// starts with among the findings.
func TestRefusedDefinitions(t *testing.T) {
	files, err := filepath.Glob("testdata/definitions/refused/*.json")
	if err != nil || len(files) == 0 {
		t.Fatalf("no fixtures: %v", err)
	}
	for _, f := range files {
		rule, _, _ := strings.Cut(strings.TrimSuffix(filepath.Base(f), ".json"), "--")
		t.Run(filepath.Base(f), func(t *testing.T) {
			_, err := domain.Compile(read(t, f))
			if !errors.Is(err, domain.ErrInvalidWorkflow) {
				t.Fatalf("err = %v, want ErrInvalidWorkflow", err)
			}
			var inv *domain.InvalidError
			if !errors.As(err, &inv) {
				t.Fatalf("err is %T", err)
			}
			if !slices.ContainsFunc(inv.Findings, func(f domain.Finding) bool { return f.Rule == rule }) {
				t.Errorf("findings %v do not include %s", inv.Findings, rule)
			}
		})
	}
}

// TestFindingsNameThePrimitiveThePlatformLacks: the refusal says what
// the vocabulary has, so the author can see there is no "legal" guard
// to be had, only generic ones.
func TestFindingsNameThePrimitiveThePlatformLacks(t *testing.T) {
	_, err := domain.Compile(read(t, "testdata/definitions/refused/unknown-primitive--a-guard-that-names-a-process.json"))
	var inv *domain.InvalidError
	if !errors.As(err, &inv) {
		t.Fatalf("err = %v", err)
	}
	f := inv.Findings[0]
	if f.Path != "guards.enough" || !strings.Contains(f.Message, `"legal_signed_off"`) || !strings.Contains(f.Message, "approvals_at_least") {
		t.Errorf("finding = %+v", f)
	}
}

func TestVersionsAreImmutableAndKeepTheirIdentity(t *testing.T) {
	d := compile(t, read(t, "testdata/definitions/vendor-then-four-eyes.json"))
	now := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	rec := domain.DefinitionRecord{ID: uuid.New(), Name: d.Name, Subject: d.Subject}

	first := domain.FirstVersion(rec, d, "person:a", now)
	if first.Number != 1 || first.Schema != domain.SchemaV1 || string(first.Document) != string(d.Document) {
		t.Fatalf("first = %+v", first)
	}
	rec.Latest = first.Number
	second, err := domain.NextVersion(rec, d, "person:b", now)
	if err != nil || second.Number != 2 || second.ID == first.ID {
		t.Fatalf("second = %+v, %v", second, err)
	}
	if loaded, err := second.Load(); err != nil || loaded.Name != d.Name {
		t.Fatalf("load: %v", err)
	}

	other := compile(t, defaults.Review())
	if _, err := domain.NextVersion(rec, other, "person:b", now); !errors.Is(err, domain.ErrRenamed) {
		t.Errorf("a version under another name: err = %v, want ErrRenamed", err)
	}
	rec.Name, rec.Subject = other.Name, domain.SubjectReleaseRequest
	if _, err := domain.NextVersion(rec, other, "person:b", now); !errors.Is(err, domain.ErrSubjectChanged) {
		t.Errorf("a version about another subject: err = %v, want ErrSubjectChanged", err)
	}
}

func TestInfoFindingsAreKeptNotRefused(t *testing.T) {
	doc := []byte(`{"schema":"glossa.workflow/v1","name":"loop","subject":"translation",
	  "chart":{"id":"loop","initial":"waiting","states":{
	    "waiting":{"entry":["tell"],"transitions":[
	      {"event":"translation.revised","target":"waiting"},
	      {"event":"translation.reviewed","target":"done"}]},
	    "done":{"type":"final"}}},
	  "actions":{"tell":{"use":"notify","to":{"role":"reviewer"}}}}`)
	d := compile(t, doc)
	if !slices.ContainsFunc(d.Findings, func(f domain.Finding) bool {
		return f.Rule == "self-transition" && f.Severity == domain.SeverityInfo
	}) {
		t.Errorf("findings = %v, want statekit's self-transition note", d.Findings)
	}
}

func TestDurations(t *testing.T) {
	for in, want := range map[string]time.Duration{
		"P3D": 72 * time.Hour, "PT4H": 4 * time.Hour, "PT30M": 30 * time.Minute,
		"P1W": 7 * 24 * time.Hour, "P1DT12H": 36 * time.Hour,
	} {
		d, err := domain.ParseDuration(in)
		if err != nil || d.Std() != want || d.String() != in {
			t.Errorf("ParseDuration(%q) = %v, %v; want %v", in, d.Std(), err, want)
		}
	}
	for _, in := range []string{"", "P", "PT", "P1M", "P1Y", "3d", "P1DT", "PT0M", "P400D", "P99999999999W"} {
		if _, err := domain.ParseDuration(in); !errors.Is(err, domain.ErrParams) {
			t.Errorf("ParseDuration(%q): err = %v, want ErrParams", in, err)
		}
	}
}
