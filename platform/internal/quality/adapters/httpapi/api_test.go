package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/felixgeelhaar/glossa/platform/internal/kernel/problem"
	"github.com/felixgeelhaar/glossa/platform/internal/quality/app"
	"github.com/felixgeelhaar/glossa/platform/internal/quality/domain"
)

func TestErrorsMapToTheDocumentedCodes(t *testing.T) {
	cases := []struct {
		err    error
		status int
		code   problem.Code
	}{
		{app.ErrProjectNotFound, 404, "not_found"},
		{app.ErrCheckRunNotFound, 404, "not_found"},
		{app.ErrWaiverNotFound, 404, "not_found"},
		{fmt.Errorf("%w: layer %q", app.ErrInvalidQuery, "nonsense"), 400, "invalid_query"},
		{domain.ErrReasonRequired, 400, "waiver_reason_required"},
		{fmt.Errorf("%w: %q", domain.ErrInvalidFingerprint, "x"), 400, "invalid_fingerprint"},
		{fmt.Errorf("%w: %q", domain.ErrInvalidScope, "tenant"), 400, "invalid_waiver_scope"},
		{domain.ErrBranchRequired, 400, "waiver_branch_required"},
		{domain.ErrExpiryInThePast, 400, "waiver_expiry_in_the_past"},
		{domain.ErrReasonTooLong, 400, "invalid_waiver"},
	}
	for _, tc := range cases {
		var d *problem.Details
		if !errors.As(mapError(tc.err), &d) || d.Status != tc.status || d.Code != tc.code {
			t.Errorf("mapError(%v) = %v, want %d %s", tc.err, d, tc.status, tc.code)
		}
	}
	other := errors.New("boom")
	if mapError(other) != other {
		t.Error("an unknown error was mapped")
	}
}

// TestFindingRendersTheWireSchema: the rendered finding is a
// glossa.finding/v1 document, and an absent member stays absent —
// MCP's read tools and `glossa check --json` share this shape
// (RFC 0005 §2.1).
func TestFindingRendersTheWireSchema(t *testing.T) {
	to := 28
	f := domain.New(domain.Finding{
		Layer: domain.LayerLength, Code: "expansion-excessive", Severity: domain.Warning,
		Message: "French is 74 % longer than the German source.",
		Subject: "checkout.pay", Detail: "ratio",
		Locus: domain.Locus{
			Message: "0192f5a1-0000-7000-8000-000000000001", Key: "checkout.pay", Locale: "fr",
			Namespace: "checkout", File: "src/PaymentFooter.vue", Line: 42, Column: 7,
			Route: "/checkout/payment", Component: "PaymentFooter",
			Span: &domain.Span{Side: domain.SideTarget, Start: 12, End: 19},
		},
		Evidence:       map[string]any{"ratio": 1.74},
		Fix:            &domain.Fix{Kind: domain.FixShorten, To: &to, Hint: "Payer maintenant"},
		SourceRevision: func() *int { r := 7; return &r }(),
	})
	b, err := json.Marshal(toFinding(f))
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got["schema"] != domain.Schema {
		t.Errorf("schema = %v", got["schema"])
	}
	if _, ok := got["waiver"]; ok {
		t.Error("a finding no waiver accepted must not carry a `waiver` member")
	}
	locus, _ := got["locus"].(map[string]any)
	if locus["line"] != float64(42) || locus["column"] != float64(7) {
		t.Errorf("locus = %v", locus)
	}
	span, _ := locus["span"].(map[string]any)
	if span["side"] != "target" || span["start"] != float64(12) {
		t.Errorf("span = %v", span)
	}

	// A waived finding names its waiver, and nothing else changes.
	id := uuid.Must(uuid.NewV7()).String()
	b, err = json.Marshal(toFinding(f.Waive(id)))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got["severity"] != string(domain.Waived) || got["waiver"] != id {
		t.Errorf("waived finding = %v", got)
	}
}

// TestEmptyLocusStaysEmpty: every locus field is optional, so a finding
// about the project alone renders an empty locus rather than a wall of
// empty strings.
func TestEmptyLocusStaysEmpty(t *testing.T) {
	b, err := json.Marshal(toLocus(domain.Locus{}))
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "{}" {
		t.Errorf("locus = %s", b)
	}
}

func TestWaiverRendersWhatItAccepts(t *testing.T) {
	expires := time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)
	w := toWaiver(app.WaiverRecord{
		Waiver: domain.Waiver{
			ID: uuid.Must(uuid.NewV7()), Fingerprint: "f_0123456789abcdef", Reason: "Login is the German term",
			Scope: domain.WaiverBranch, Ref: "main", SourceRevision: 7, CreatedBy: "person:1",
			CreatedAt: time.Now().UTC(), ExpiresAt: &expires,
		},
		Accepts: app.FindingSummary{Layer: string(domain.LayerTerminology), Code: "term_missing", Locale: "de", Key: "a.b"},
		Active:  true,
	})
	if !w.Active || w.Ref == nil || *w.Ref != "main" || w.ExpiresAt == nil {
		t.Fatalf("waiver = %+v", w)
	}
	if w.Accepts == nil || *w.Accepts.Code != "term_missing" || string(*w.Accepts.Layer) != string(domain.LayerTerminology) {
		t.Errorf("accepts = %+v", w.Accepts)
	}
	// A waiver whose finding no longer occurs anywhere carries none.
	bare := toWaiver(app.WaiverRecord{Waiver: domain.Waiver{ID: uuid.Must(uuid.NewV7()), Scope: domain.WaiverProject}})
	if bare.Accepts != nil || bare.Ref != nil || bare.RevokedAt != nil {
		t.Errorf("bare waiver = %+v", bare)
	}
}

func TestPathIDsAreNotFound(t *testing.T) {
	if _, err := projectID("not-a-uuid"); err == nil {
		t.Error("a malformed project ID was accepted")
	}
	if _, err := optionalRunID(nil); err != nil {
		t.Errorf("no run parameter: %v", err)
	}
	empty := ""
	if id, err := optionalRunID(&empty); err != nil || id != uuid.Nil {
		t.Errorf("an empty run parameter: %v, %v", id, err)
	}
	bad := "nope"
	var d *problem.Details
	if _, err := optionalRunID(&bad); !errors.As(err, &d) || d.Status != 404 {
		t.Errorf("a malformed run parameter: %v", err)
	}
}
