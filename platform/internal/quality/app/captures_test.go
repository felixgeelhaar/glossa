package app_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/felixgeelhaar/glossa/platform/internal/kernel/checkpolicy"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/pagination"
	"github.com/felixgeelhaar/glossa/platform/internal/quality/app"
	"github.com/felixgeelhaar/glossa/platform/internal/quality/domain"
)

// A capture upload's visual findings (RFC 0005 §5, §13 wave 4). The
// probe pass leaves two members of the finding open, because the page
// cannot know them; these tests are about what the ingest puts there.

var (
	capture     = uuid.MustParse("0192f5c2-0000-7000-8000-00000000c0de")
	messageID   = uuid.MustParse("0192f5a1-0000-7000-8000-00000000beef")
	captureTime = time.Date(2026, 9, 30, 9, 0, 0, 0, time.UTC)
)

func upload(project uuid.UUID, findings ...app.CaptureFinding) app.RecordVisualFindings {
	return app.RecordVisualFindings{
		Project: project, Ref: "feat/checkout-copy", Commit: "9f2c1e7a4b3d5c6e8f0a1b2c3d4e5f6a7b8c9d0e",
		StartedAt: captureTime,
		Captures: []app.CaptureFindings{{
			Capture: capture, Locale: "fr", Findings: findings,
		}},
	}
}

func clipped() app.CaptureFinding {
	return app.CaptureFinding{
		Code: "text-clipped", Message: messageID, Key: "checkout.pay", Region: "r_0",
		Explanation: "Clipped: 412x20 px of text in 358x20 px.",
		Evidence:    map[string]any{"box": []any{358, 20}},
	}
}

// The fingerprint is the server's, over the catalog message ID, and the
// capture is minted here: a fingerprint a page computed would hash the
// key (a browser has no message ID) and would not be the one a waiver
// was written against, so the waiver would silently stop applying.
func TestRecordVisualFindingsCompletesWhatThePageCannotKnow(t *testing.T) {
	store := recordingStore()
	svc, project := serviceFor(store)

	out, err := svc.RecordVisualFindings(writeCtx(t), upload(project, clipped()))
	if err != nil {
		t.Fatal(err)
	}
	if out.Findings != 1 || out.Run == uuid.Nil {
		t.Fatalf("recorded = %+v", out)
	}
	if len(store.inserted) != 1 {
		t.Fatalf("stored %d findings", len(store.inserted))
	}
	f := store.inserted[0]
	want := domain.Fingerprint(domain.LayerVisual, "text-clipped",
		domain.Locus{Message: messageID.String(), Locale: "fr"}, "")
	if f.Fingerprint != want {
		t.Errorf("fingerprint = %s, want the server's over the message ID (%s)", f.Fingerprint, want)
	}
	if f.Locus.Capture != capture.String() || f.Locus.Region != "r_0" {
		t.Errorf("locus = %+v, want the capture minted beside the region", f.Locus)
	}
	// One capture is one (route, viewport, locale), so a finding that
	// names no locale is about the capture's.
	if f.Locus.Locale != "fr" || f.Locus.Message != messageID.String() || f.Locus.Key != "checkout.pay" {
		t.Errorf("locus = %+v", f.Locus)
	}
	if f.Schema != domain.Schema || f.Layer != domain.LayerVisual || f.Severity != domain.Warning {
		t.Errorf("finding = %+v, want a visual warning of glossa.finding/v1", f)
	}
	if store.recorded.Trigger != domain.TriggerCapture || store.recorded.Ref != "feat/checkout-copy" ||
		len(store.recorded.Layers) != 1 || store.recorded.Layers[0] != domain.LayerVisual {
		t.Errorf("run = %+v, want one capture-triggered run of the visual layer", store.recorded)
	}
	if !store.recorded.StartedAt.Equal(captureTime) {
		t.Errorf("started at %s, want the capture session's %s", store.recorded.StartedAt, captureTime)
	}
}

// A finding about a key the catalog doesn't know is still true: it is
// stored, and its fingerprint falls back to the key exactly as an
// offline check's does.
func TestRecordVisualFindingsKeepsAFindingAboutAnUnknownKey(t *testing.T) {
	store := recordingStore()
	svc, project := serviceFor(store)

	f := clipped()
	f.Message, f.Key, f.Region = uuid.Nil, "checkout.gone", ""
	if _, err := svc.RecordVisualFindings(writeCtx(t), upload(project, f)); err != nil {
		t.Fatal(err)
	}
	got := store.inserted[0]
	want := domain.Fingerprint(domain.LayerVisual, "text-clipped", domain.Locus{Key: "checkout.gone", Locale: "fr"}, "")
	if got.Fingerprint != want || got.Locus.Message != "" {
		t.Errorf("finding = %+v, want the fingerprint over the key (%s)", got, want)
	}
	// Nothing names a region, so nothing names a capture region either —
	// but the capture itself is still where this was seen.
	if got.Locus.Region != "" || got.Locus.Capture != capture.String() {
		t.Errorf("locus = %+v", got.Locus)
	}
}

// `off` means the project does not compute the layer, so an upload
// carrying findings of a layer nobody asked for stores nothing: a
// project doesn't pay for a layer it ignores (RFC 0005 §4.1).
func TestRecordVisualFindingsStoresNothingWhenThePolicySwitchesVisualOff(t *testing.T) {
	store := recordingStore()
	svc, catalog, project := serviceAndCatalog(store)
	catalog.policy = checkpolicy.Policy{
		Version: 7,
		Rules: []checkpolicy.Rule{{
			Selector: checkpolicy.Selector{Layer: string(domain.LayerVisual)}, Severity: checkpolicy.Off,
		}},
	}

	out, err := svc.RecordVisualFindings(writeCtx(t), upload(project, clipped()))
	if err != nil {
		t.Fatal(err)
	}
	if out.Run != uuid.Nil || out.Findings != 0 {
		t.Errorf("recorded = %+v, want nothing", out)
	}
	if len(store.inserted) != 0 || store.recorded.ID != uuid.Nil {
		t.Errorf("stored %d findings in run %v", len(store.inserted), store.recorded.ID)
	}
}

// The policy is the evaluator for these as for every other finding: a
// rule that raises `visual` to an error is what the run records, and
// the run's verdict follows from it.
func TestRecordVisualFindingsGradesAgainstTheProjectPolicy(t *testing.T) {
	store := recordingStore()
	svc, catalog, project := serviceAndCatalog(store)
	catalog.policy = checkpolicy.Policy{
		Version: 9,
		Rules: []checkpolicy.Rule{{
			Selector: checkpolicy.Selector{Layer: string(domain.LayerVisual)}, Severity: checkpolicy.Error,
		}},
	}

	if _, err := svc.RecordVisualFindings(writeCtx(t), upload(project, clipped())); err != nil {
		t.Fatal(err)
	}
	if store.inserted[0].Severity != domain.Error {
		t.Errorf("severity = %s, want the policy's", store.inserted[0].Severity)
	}
	if store.recorded.PolicyVersion != 9 || store.recorded.Conclusion != domain.ConclusionFailure {
		t.Errorf("run = version %d, %s", store.recorded.PolicyVersion, store.recorded.Conclusion)
	}
}

// A run holds at most 10 000 findings (RFC 0005 §10): a page that
// floods findings cannot flood storage.
func TestRecordVisualFindingsRefusesMoreThanARunMayHold(t *testing.T) {
	store := recordingStore()
	svc, project := serviceFor(store)

	in := upload(project)
	in.Captures[0].Findings = make([]app.CaptureFinding, app.MaxRunFindings+1)
	for i := range in.Captures[0].Findings {
		in.Captures[0].Findings[i] = clipped()
	}
	if _, err := svc.RecordVisualFindings(writeCtx(t), in); !errors.Is(err, app.ErrTooManyFindings) {
		t.Errorf("error = %v, want ErrTooManyFindings", err)
	}
	if len(store.inserted) != 0 {
		t.Errorf("stored %d findings", len(store.inserted))
	}
}

// Reading back: the findings of one capture, and of one region of it.
func TestListCaptureFindings(t *testing.T) {
	store := recordingStore()
	store.rows = []app.FindingRecord{
		{Finding: domain.New(domain.Finding{
			Layer: domain.LayerVisual, Code: "text-clipped", Severity: domain.Warning,
			Locus: domain.Locus{Key: "checkout.pay", Locale: "fr", Capture: capture.String(), Region: "r_0"},
		}), SortKey: "run\x01r_0\x01a"},
		{Finding: domain.New(domain.Finding{
			Layer: domain.LayerVisual, Code: "region-overlap", Severity: domain.Warning,
			Locus: domain.Locus{Key: "cart.total", Locale: "fr", Capture: capture.String(), Region: "r_3"},
		}), SortKey: "run\x01r_3\x01b"},
	}
	svc, project := serviceFor(store)

	items, next, err := svc.ListCaptureFindings(readCtx(t), project,
		app.CaptureFindingQuery{Capture: capture, Region: "r_3"}, pageOf(pagination.DefaultPageSize))
	if err != nil {
		t.Fatal(err)
	}
	if store.lastCapture != capture || store.lastRegion != "r_3" {
		t.Errorf("asked for capture %s region %q", store.lastCapture, store.lastRegion)
	}
	if len(items) != 2 || next != nil {
		t.Fatalf("items = %d, next = %v (the fake filters nothing; the query does)", len(items), next)
	}
	if items[0].Locus.Capture != capture.String() {
		t.Errorf("finding = %+v", items[0])
	}
}

// A capture the path cannot name is a 404, not an empty page that looks
// like a clean screenshot.
func TestListCaptureFindingsNeedsACapture(t *testing.T) {
	svc, project := serviceFor(recordingStore())
	_, _, err := svc.ListCaptureFindings(readCtx(t), project,
		app.CaptureFindingQuery{}, pageOf(pagination.DefaultPageSize))
	if !errors.Is(err, app.ErrCaptureNotFound) {
		t.Errorf("error = %v, want ErrCaptureNotFound", err)
	}
}
