// Package metrics records the Context context in Prometheus (RFC 0004
// §11): usage uploads and the usages they carried, by collector, and
// each project's context coverage — the share of its active messages
// with a current usage on the default branch.
package metrics

import (
	"errors"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/felixgeelhaar/glossa/platform/internal/context/app"
	"github.com/felixgeelhaar/glossa/platform/internal/context/domain"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/tenancy"
)

// Prometheus implements app.Metrics.
type Prometheus struct {
	builds   *prometheus.CounterVec
	usages   *prometheus.CounterVec
	unknown  *prometheus.CounterVec
	coverage *prometheus.GaugeVec
}

var _ app.Metrics = (*Prometheus)(nil)

// New registers the collectors on reg (a private registry when nil).
// Re-registering reuses the existing collectors.
func New(reg prometheus.Registerer) *Prometheus {
	if reg == nil {
		reg = prometheus.NewRegistry()
	}
	return &Prometheus{
		builds: register(reg, prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "glossa_context_builds_total",
			Help: "Usage uploads, by source (plugin, extract, runtime, capture) and outcome (stored, replayed).",
		}, []string{"source", "outcome"})),
		usages: register(reg, prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "glossa_context_usages_ingested_total",
			Help: "Usages stored by uploads, by source.",
		}, []string{"source"})),
		unknown: register(reg, prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "glossa_context_unknown_keys_total",
			Help: "Stored usages whose key the catalog didn't know, by source.",
		}, []string{"source"})),
		coverage: register(reg, prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "glossa_context_coverage_ratio",
			Help: "Share of a project's active messages with a current usage on the default branch (1 with none active), measured after each default-branch upload. Each instance reports what it measured last; take the max across instances.",
		}, []string{"tenant", "project"})),
	}
}

// BuildIngested implements app.Metrics.
func (p *Prometheus) BuildIngested(source domain.Source, usages, unknownKeys int, replayed bool) {
	if replayed {
		p.builds.WithLabelValues(string(source), "replayed").Inc()
		return
	}
	p.builds.WithLabelValues(string(source), "stored").Inc()
	p.usages.WithLabelValues(string(source)).Add(float64(usages))
	p.unknown.WithLabelValues(string(source)).Add(float64(unknownKeys))
}

// Coverage implements app.Metrics.
func (p *Prometheus) Coverage(tenant tenancy.ID, project uuid.UUID, active, used int) {
	ratio := 1.0
	if active > 0 {
		ratio = float64(used) / float64(active)
	}
	p.coverage.WithLabelValues(tenant.String(), project.String()).Set(ratio)
}

func register[C prometheus.Collector](reg prometheus.Registerer, c C) C {
	if err := reg.Register(c); err != nil {
		var are prometheus.AlreadyRegisteredError
		if errors.As(err, &are) {
			if existing, ok := are.ExistingCollector.(C); ok {
				return existing
			}
		}
		panic(err)
	}
	return c
}
