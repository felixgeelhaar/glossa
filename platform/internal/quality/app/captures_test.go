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
	"github.com/felixgeelhaar/glossa/platform/internal/quality/layers"
)

// A capture upload's visual findings (RFC 0005 §5, §13 wave 4). The
// probe pass leaves two members of the finding open, because the page
// cannot know them; these tests are about what the ingest puts there.

var (
	capture = uuid.MustParse("0192f5c2-0000-7000-8000-00000000c0de")
	// before is the capture this scope showed last: the previous
	// sighting the two-sighting rule counts against.
	before      = uuid.MustParse("0192f5c1-0000-7000-8000-00000000cafe")
	messageID   = uuid.MustParse("0192f5a1-0000-7000-8000-00000000beef")
	captureTime = time.Date(2026, 9, 30, 9, 0, 0, 0, time.UTC)
)

func upload(project uuid.UUID, findings ...app.CaptureFinding) app.RecordVisualFindings {
	return app.RecordVisualFindings{
		Project: project, Ref: "feat/checkout-copy", Commit: "9f2c1e7a4b3d5c6e8f0a1b2c3d4e5f6a7b8c9d0e",
		StartedAt: captureTime,
		Captures: []app.CaptureFindings{{
			Capture: capture, Route: "/checkout", Width: 390, Height: 844, Locale: "fr", Findings: findings,
		}},
	}
}

// clippedPrint is the fingerprint the server computes for clipped(): the
// identity a second sighting has to match.
func clippedPrint() string {
	return domain.Fingerprint(domain.LayerVisual, "text-clipped",
		domain.Locus{Message: messageID.String(), Locale: "fr"}, "")
}

// seenBefore makes the previous capture of the scope one that showed
// the same fingerprints.
func seenBefore(store *fakeStore, in app.RecordVisualFindings, fingerprints ...string) app.RecordVisualFindings {
	in.Captures[0].Previous = before
	store.sighted = map[uuid.UUID][]string{before: fingerprints}
	return in
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

// strictVisual raises the visual layer to an error.
func strictVisual() checkpolicy.Policy {
	return checkpolicy.Policy{
		Version: 9,
		Rules: []checkpolicy.Rule{{
			Selector: checkpolicy.Selector{Layer: string(domain.LayerVisual)}, Severity: checkpolicy.Error,
		}},
	}
}

// The policy is the evaluator for these as for every other finding: a
// rule that raises `visual` to an error is what the run records, and
// the run's verdict follows from it — once the finding is evidence.
// The second sighting comes from the findings stored against the
// previous capture of the same (route, viewport, locale).
func TestRecordVisualFindingsGradesAgainstTheProjectPolicy(t *testing.T) {
	store := recordingStore()
	svc, catalog, project := serviceAndCatalog(store)
	catalog.policy = strictVisual()

	in := seenBefore(store, upload(project, clipped()), clippedPrint())
	if _, err := svc.RecordVisualFindings(writeCtx(t), in); err != nil {
		t.Fatal(err)
	}
	if store.lastPrevious != before {
		t.Errorf("read the sightings of %s, want the previous capture %s", store.lastPrevious, before)
	}
	if got := store.inserted[0].Sightings(); got != checkpolicy.DefaultVisualThresholds().PromoteAfterSightings {
		t.Errorf("sightings = %d, want the finding counted as seen before", got)
	}
	if store.inserted[0].Severity != domain.Error {
		t.Errorf("severity = %s, want the policy's", store.inserted[0].Severity)
	}
	if store.recorded.PolicyVersion != 9 || store.recorded.Conclusion != domain.ConclusionFailure {
		t.Errorf("run = version %d, %s", store.recorded.PolicyVersion, store.recorded.Conclusion)
	}
}

// The flake-control rule of RFC 0005 §5.2, from the other side: one
// sighting is not evidence, and no policy can raise it. Headless
// Chrome's text metrics move with font availability, so a build that
// went red on the first sighting would go red on a font.
func TestRecordVisualFindingsLeavesAFirstSightingAWarning(t *testing.T) {
	cases := map[string]func(*fakeStore, app.RecordVisualFindings) app.RecordVisualFindings{
		"a scope nobody has captured before": func(_ *fakeStore, in app.RecordVisualFindings) app.RecordVisualFindings {
			return in
		},
		"a previous capture that showed something else": func(s *fakeStore, in app.RecordVisualFindings) app.RecordVisualFindings {
			return seenBefore(s, in, "f_0000000000000000")
		},
	}
	for why, setUp := range cases {
		t.Run(why, func(t *testing.T) {
			store := recordingStore()
			svc, catalog, project := serviceAndCatalog(store)
			catalog.policy = strictVisual()

			if _, err := svc.RecordVisualFindings(writeCtx(t), setUp(store, upload(project, clipped()))); err != nil {
				t.Fatal(err)
			}
			if got := store.inserted[0].Sightings(); got != 1 {
				t.Errorf("sightings = %d, want 1", got)
			}
			if store.inserted[0].Severity != domain.Warning || !store.inserted[0].Provisional() {
				t.Errorf("severity = %s, want a warning no rule can raise", store.inserted[0].Severity)
			}
			if store.recorded.Conclusion != domain.ConclusionSuccess {
				t.Errorf("conclusion = %s, want success", store.recorded.Conclusion)
			}
		})
	}
}

// Cross-surface identity (RFC 0005 §2.1): the fingerprint `glossa
// capture --check` computes for a visual finding and the one the
// capture ingest computes for the same finding must be the same, or a
// waiver made on one surface silently stops matching on the other.
//
// Both now run the same rule — layers.PromoteVisual seals and
// fingerprints on both sides — so the route, the viewport, the region,
// the capture, the evidence and the sighting count provably do not
// disturb the identity, and a locale the page left open is filled from
// the capture's scope the same way on both.
//
// The one thing that does differ is deliberate and documented in
// domain.Fingerprint: the hash is over the *catalog message ID* where
// the caller has one and over the key where it does not. The server
// always has the ID; `glossa check` offline has only keys. See the
// second case: it is the identity a rename survives, and closing the
// gap means giving the CLI's snapshot the message IDs, not changing
// what the server hashes.
func TestTheCLIAndTheIngestFingerprintAVisualFindingTheSameWay(t *testing.T) {
	scope := layers.VisualScope{Route: "/checkout", Width: 390, Height: 844, Locale: "fr"}
	// What the page wrote: no fingerprint, no capture, no locale.
	probe := domain.Finding{
		Layer: domain.LayerVisual, Code: "text-clipped", Severity: domain.Warning,
		Locus:   domain.Locus{Key: "checkout.pay", Region: "r_0"},
		Message: "Clipped: 412x20 px of text in 358x20 px.",
	}
	cli, _ := layers.PromoteVisual(nil, []layers.Probed{{Scope: scope, Findings: []domain.Finding{probe}}},
		checkpolicy.DefaultVisualThresholds())
	if len(cli.Findings) != 1 {
		t.Fatalf("the CLI found %d", len(cli.Findings))
	}

	ingested := func(message uuid.UUID) domain.Finding {
		t.Helper()
		store := recordingStore()
		svc, project := serviceFor(store)
		in := upload(project, app.CaptureFinding{
			Code: "text-clipped", Message: message, Key: "checkout.pay", Region: "r_0",
			Explanation: probe.Message,
		})
		if _, err := svc.RecordVisualFindings(writeCtx(t), in); err != nil {
			t.Fatal(err)
		}
		return store.inserted[0]
	}

	// A key the catalog does not know: both surfaces hash the key, and
	// the fingerprints are equal.
	if got := ingested(uuid.Nil); got.Fingerprint != cli.Findings[0].Fingerprint {
		t.Errorf("the ingest computed %s and the CLI %s for the same finding", got.Fingerprint, cli.Findings[0].Fingerprint)
	}
	// A key the catalog knows: the server hashes the message ID, which
	// is the identity RFC 0005 §2.1 names and the one a rename
	// survives. An offline check cannot compute it, so the two differ
	// until the CLI's snapshot carries message IDs.
	resolved := ingested(messageID)
	if resolved.Fingerprint == cli.Findings[0].Fingerprint {
		t.Error("the server hashed the key; it has the message ID and should hash that")
	}
	if want := clippedPrint(); resolved.Fingerprint != want {
		t.Errorf("fingerprint = %s, want %s", resolved.Fingerprint, want)
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
