// Package metrics records the Quality context in Prometheus (RFC 0005
// §11): the findings each layer produced, by code and severity; check
// runs by what asked for them and what they concluded; and what became
// of waiver requests.
//
// Nothing counted a finding before M4. Label values are allowlisted —
// the layer is one of the ten, the severity one of the three, the code
// is dropped past a bound — because a code comes from a layer and a
// dashboard that melts under one project's vocabulary is worse than one
// number fewer.
package metrics

import (
	"errors"
	"slices"
	"sync"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/felixgeelhaar/glossa/platform/internal/quality/app"
	"github.com/felixgeelhaar/glossa/platform/internal/quality/domain"
)

// MaxCodes bounds the distinct codes one process labels findings with;
// everything past it is counted as "other", so one tenant's vocabulary
// can't unbound the series.
const MaxCodes = 200

// Prometheus implements app.Metrics.
type Prometheus struct {
	findings *prometheus.CounterVec
	runs     *prometheus.CounterVec
	waivers  *prometheus.CounterVec

	mu    sync.Mutex
	codes map[string]struct{}
}

var _ app.Metrics = (*Prometheus)(nil)

// New registers the collectors on reg (a private registry when nil).
// Re-registering reuses the existing collectors.
func New(reg prometheus.Registerer) *Prometheus {
	if reg == nil {
		reg = prometheus.NewRegistry()
	}
	return &Prometheus{
		findings: register(reg, prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "glossa_quality_findings_total",
			Help: "Findings stored with a check run, by layer, code and the severity their layer emitted (a waiver is applied on read, so `waived` never appears here).",
		}, []string{"layer", "code", "severity"})),
		runs: register(reg, prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "glossa_quality_check_runs_total",
			Help: "Check runs stored, by trigger (cli, pull_request, write, capture, api) and conclusion (success, failure, neutral).",
		}, []string{"trigger", "conclusion"})),
		waivers: register(reg, prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "glossa_quality_waivers_total",
			Help: "Waiver decisions, by outcome (created, updated, revoked, refused).",
		}, []string{"outcome"})),
		codes: map[string]struct{}{},
	}
}

// FindingRecorded implements app.Metrics.
func (p *Prometheus) FindingRecorded(layer domain.Layer, code string, severity domain.Severity) {
	p.findings.WithLabelValues(known(string(layer), layers()), p.knownCode(code),
		known(string(severity), []string{string(domain.Error), string(domain.Warning), string(domain.Waived)})).Inc()
}

// CheckRunRecorded implements app.Metrics.
func (p *Prometheus) CheckRunRecorded(trigger domain.Trigger, conclusion domain.Conclusion) {
	p.runs.WithLabelValues(known(string(trigger), triggers()), known(string(conclusion), conclusions())).Inc()
}

// WaiverDecided implements app.Metrics.
func (p *Prometheus) WaiverDecided(outcome string) {
	p.waivers.WithLabelValues(known(outcome, []string{app.WaiverCreated, app.WaiverUpdated, app.WaiverRevoked, app.WaiverRefused})).Inc()
}

// knownCode keeps a code as a label until MaxCodes distinct ones have
// been seen, and buckets the rest.
func (p *Prometheus) knownCode(code string) string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, ok := p.codes[code]; ok {
		return code
	}
	if code == "" || len(p.codes) >= MaxCodes {
		return "other"
	}
	p.codes[code] = struct{}{}
	return code
}

func known(v string, allowed []string) string {
	if slices.Contains(allowed, v) {
		return v
	}
	return "other"
}

func layers() []string {
	out := make([]string, len(domain.Layers))
	for i, l := range domain.Layers {
		out[i] = string(l)
	}
	return out
}

func triggers() []string {
	out := make([]string, len(domain.Triggers))
	for i, t := range domain.Triggers {
		out[i] = string(t)
	}
	return out
}

func conclusions() []string {
	return []string{string(domain.ConclusionSuccess), string(domain.ConclusionFailure), string(domain.ConclusionNeutral)}
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
