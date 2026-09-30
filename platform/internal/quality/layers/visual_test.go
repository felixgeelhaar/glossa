package layers_test

import (
	"testing"

	"github.com/felixgeelhaar/glossa/platform/internal/kernel/checkpolicy"
	"github.com/felixgeelhaar/glossa/platform/internal/quality/domain"
	"github.com/felixgeelhaar/glossa/platform/internal/quality/layers"
)

// The two-sighting rule (RFC 0005 §5.2), which is the whole reason the
// visual layer has a promotion step: a finding measured once in a
// headless browser is not evidence, and a finding measured twice in a
// row in the same place is.

func phone() layers.VisualScope {
	return layers.VisualScope{Route: "/checkout", Width: 390, Height: 844, Locale: "de"}
}

func clipped(key string) domain.Finding {
	return domain.Finding{
		Layer: domain.LayerVisual, Code: "text-clipped", Severity: domain.Warning,
		Locus: domain.Locus{Key: key, Region: "r_3"}, Message: "Clipped: 210×20 px of text in 148×20 px.",
	}
}

func thresholds() checkpolicy.VisualThresholds { return checkpolicy.DefaultVisualThresholds() }

func only(t *testing.T, v layers.Visual) domain.Finding {
	t.Helper()
	if len(v.Findings) != 1 {
		t.Fatalf("findings = %d, want 1: %+v", len(v.Findings), v.Findings)
	}
	return v.Findings[0]
}

// A finding nobody has seen before is a warning that no policy can
// raise, and the run remembers it for the next one.
func TestAFirstSightingIsProvisional(t *testing.T) {
	v, next := layers.PromoteVisual(nil, []layers.Probed{{Scope: phone(), Findings: []domain.Finding{clipped("checkout.pay")}}}, thresholds())
	f := only(t, v)
	switch {
	case f.Severity != domain.Warning:
		t.Errorf("severity = %q, want warning", f.Severity)
	case f.Sightings() != 1:
		t.Errorf("sightings = %d, want 1", f.Sightings())
	case !f.Provisional():
		t.Error("a first sighting is not provisional")
	case f.Fingerprint == "":
		t.Error("the layer left the finding without an identity")
	case f.Locus.Route != "/checkout" || f.Locus.Locale != "de":
		t.Errorf("locus = %+v, want the capture's route and locale", f.Locus)
	}
	if got := next[phone().Key()]; len(got) != 1 || got[0] != f.Fingerprint {
		t.Errorf("remembered %v, want %q", got, f.Fingerprint)
	}
}

// The same fingerprint in the next capture of the same (route,
// viewport, locale) is the second consecutive sighting, and the finding
// becomes eligible for error.
func TestASecondConsecutiveSightingIsPromoted(t *testing.T) {
	probed := []layers.Probed{{Scope: phone(), Findings: []domain.Finding{clipped("checkout.pay")}}}
	_, after := layers.PromoteVisual(nil, probed, thresholds())
	v, _ := layers.PromoteVisual(after, probed, thresholds())
	f := only(t, v)
	if f.Sightings() != 2 || f.Provisional() {
		t.Fatalf("second sighting = %d sightings, provisional %v", f.Sightings(), f.Provisional())
	}
	// Eligible, not failed: without a rule it is still a warning.
	if f.Severity != domain.Warning {
		t.Errorf("severity = %q; promotion makes a finding eligible for error, not an error", f.Severity)
	}
	p := checkpolicy.Policy{Rules: []checkpolicy.Rule{
		{Selector: checkpolicy.Selector{Layer: "visual"}, Severity: checkpolicy.Error},
	}}
	e := domain.Evaluate(p, "", v.Findings)
	if e.Passed() {
		t.Error("a promoted finding didn't fail a policy that raises the visual layer")
	}
	first, _ := layers.PromoteVisual(nil, probed, thresholds())
	if !domain.Evaluate(p, "", first.Findings).Passed() {
		t.Error("a first sighting failed the same policy")
	}
}

// "Consecutive" means consecutive: a capture that doesn't show the
// finding starts its count again.
func TestAMissedCaptureResetsTheCount(t *testing.T) {
	scope := phone()
	probed := []layers.Probed{{Scope: scope, Findings: []domain.Finding{clipped("checkout.pay")}}}
	_, after := layers.PromoteVisual(nil, probed, thresholds())
	_, clean := layers.PromoteVisual(after, []layers.Probed{{Scope: scope}}, thresholds())
	if len(clean[scope.Key()]) != 0 {
		t.Fatalf("a clean capture remembered %v", clean[scope.Key()])
	}
	v, _ := layers.PromoteVisual(clean, probed, thresholds())
	if f := only(t, v); f.Sightings() != 1 || !f.Provisional() {
		t.Errorf("after a clean capture = %d sightings, provisional %v", f.Sightings(), f.Provisional())
	}
}

// The scope is (route, viewport, locale): the same finding at another
// viewport is another sighting of another thing, because the two
// viewports lay the page out differently.
func TestSightingsAreCountedPerRouteViewportAndLocale(t *testing.T) {
	desktop := phone()
	desktop.Width, desktop.Height = 1280, 800
	_, after := layers.PromoteVisual(nil, []layers.Probed{{Scope: phone(), Findings: []domain.Finding{clipped("checkout.pay")}}}, thresholds())
	v, _ := layers.PromoteVisual(after, []layers.Probed{{Scope: desktop, Findings: []domain.Finding{clipped("checkout.pay")}}}, thresholds())
	if f := only(t, v); f.Sightings() != 1 {
		t.Errorf("another viewport = %d sightings, want 1", f.Sightings())
	}
}

// One problem is one finding, however many captures show it, and a run
// that confirmed it anywhere confirmed it.
func TestOneFingerprintIsReportedOnceWithItsHighestCount(t *testing.T) {
	desktop := phone()
	desktop.Width, desktop.Height = 1280, 800
	both := []layers.Probed{
		{Scope: phone(), Findings: []domain.Finding{clipped("checkout.pay")}},
		{Scope: desktop, Findings: []domain.Finding{clipped("checkout.pay")}},
	}
	_, after := layers.PromoteVisual(nil, both[1:], thresholds())
	v, _ := layers.PromoteVisual(after, both, thresholds())
	if f := only(t, v); f.Sightings() != 2 {
		t.Errorf("sightings = %d, want the highest of the two scopes", f.Sightings())
	}
}

// RFC 0005 §10: a capture reports at most MaxFindingsPerCapture.
func TestACaptureIsCappedAtTheThreshold(t *testing.T) {
	th := thresholds()
	th.MaxFindingsPerCapture = 2
	var fs []domain.Finding
	for _, key := range []string{"a", "b", "c", "d"} {
		fs = append(fs, clipped(key))
	}
	v, _ := layers.PromoteVisual(nil, []layers.Probed{{Scope: phone(), Findings: fs}}, th)
	if len(v.Findings) != 2 {
		t.Errorf("findings = %d, want the cap of 2", len(v.Findings))
	}
}
