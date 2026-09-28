package metrics

import (
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/felixgeelhaar/glossa/platform/internal/quality/app"
	"github.com/felixgeelhaar/glossa/platform/internal/quality/domain"
)

func TestCountsWhatItSees(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := New(reg)
	m.FindingRecorded(domain.LayerParity, "missing-argument", domain.Error)
	m.FindingRecorded(domain.LayerParity, "missing-argument", domain.Error)
	m.CheckRunRecorded(domain.TriggerPullRequest, domain.ConclusionFailure)
	m.WaiverDecided(app.WaiverCreated)

	if got := testutil.ToFloat64(m.findings.WithLabelValues("parity", "missing-argument", "error")); got != 2 {
		t.Errorf("findings = %v, want 2", got)
	}
	if got := testutil.ToFloat64(m.runs.WithLabelValues("pull_request", "failure")); got != 1 {
		t.Errorf("runs = %v, want 1", got)
	}
	if got := testutil.ToFloat64(m.waivers.WithLabelValues("created")); got != 1 {
		t.Errorf("waivers = %v, want 1", got)
	}
}

// TestLabelsAreBounded: a layer, severity, trigger or outcome outside
// the vocabulary becomes "other", and codes stop being labels past
// MaxCodes — a dashboard that melts under one project's vocabulary is
// worse than one number fewer.
func TestLabelsAreBounded(t *testing.T) {
	m := New(nil)
	m.FindingRecorded(domain.Layer("invented"), "a-code", domain.Severity("loud"))
	if got := testutil.ToFloat64(m.findings.WithLabelValues("other", "a-code", "other")); got != 1 {
		t.Errorf("an unknown layer and severity = %v", got)
	}
	m.CheckRunRecorded(domain.Trigger("nope"), domain.Conclusion("maybe"))
	if got := testutil.ToFloat64(m.runs.WithLabelValues("other", "other")); got != 1 {
		t.Errorf("an unknown trigger and conclusion = %v", got)
	}
	m.WaiverDecided("whatever")
	if got := testutil.ToFloat64(m.waivers.WithLabelValues("other")); got != 1 {
		t.Errorf("an unknown outcome = %v", got)
	}

	for i := range MaxCodes + 50 {
		m.FindingRecorded(domain.LayerSource, "code-"+strings.Repeat("x", i%7)+string(rune('a'+i%26))+string(rune('a'+i/26)), domain.Warning)
	}
	if got := testutil.ToFloat64(m.findings.WithLabelValues("source", "other", "warning")); got == 0 {
		t.Error("codes past MaxCodes should be bucketed as other")
	}
	if len(m.codes) > MaxCodes {
		t.Errorf("%d codes kept, want at most %d", len(m.codes), MaxCodes)
	}
}

// TestRegisteringTwiceReusesTheCollectors, so two services on one
// registry don't panic.
func TestRegisteringTwiceReusesTheCollectors(t *testing.T) {
	reg := prometheus.NewRegistry()
	a, b := New(reg), New(reg)
	a.WaiverDecided(app.WaiverRevoked)
	if got := testutil.ToFloat64(b.waivers.WithLabelValues("revoked")); got != 1 {
		t.Errorf("the second registration got its own collector: %v", got)
	}
}
