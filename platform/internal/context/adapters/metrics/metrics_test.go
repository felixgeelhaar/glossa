package metrics_test

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/felixgeelhaar/glossa/platform/internal/context/adapters/metrics"
	"github.com/felixgeelhaar/glossa/platform/internal/context/domain"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/tenancy"
)

func TestPrometheusRecordsUploadsAndCoverage(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := metrics.New(reg)
	if metrics.New(reg) == nil {
		t.Fatal("registering twice reuses the collectors")
	}
	m.BuildIngested(domain.SourcePlugin, 120, 3, false)
	m.BuildIngested(domain.SourcePlugin, 120, 3, true)
	m.BuildIngested(domain.SourceExtract, 7, 0, false)
	tenant, project := tenancy.NewID(), uuid.New()
	m.Coverage(tenant, project, 4, 3)
	empty := uuid.New()
	m.Coverage(tenant, empty, 0, 0)

	want := `
# HELP glossa_context_usages_ingested_total Usages stored by uploads, by source.
# TYPE glossa_context_usages_ingested_total counter
glossa_context_usages_ingested_total{source="extract"} 7
glossa_context_usages_ingested_total{source="plugin"} 120
# HELP glossa_context_builds_total Usage uploads, by source (plugin, extract, runtime, capture) and outcome (stored, replayed).
# TYPE glossa_context_builds_total counter
glossa_context_builds_total{outcome="replayed",source="plugin"} 1
glossa_context_builds_total{outcome="stored",source="extract"} 1
glossa_context_builds_total{outcome="stored",source="plugin"} 1
`
	if err := testutil.GatherAndCompare(reg, strings.NewReader(want),
		"glossa_context_usages_ingested_total", "glossa_context_builds_total"); err != nil {
		t.Error(err)
	}
	g, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	ratios := map[string]float64{}
	for _, f := range g {
		if f.GetName() != "glossa_context_coverage_ratio" {
			continue
		}
		for _, s := range f.GetMetric() {
			for _, l := range s.GetLabel() {
				if l.GetName() == "project" {
					ratios[l.GetValue()] = s.GetGauge().GetValue()
				}
			}
		}
	}
	if ratios[project.String()] != 0.75 || ratios[empty.String()] != 1 {
		t.Errorf("coverage = %v", ratios)
	}
}
