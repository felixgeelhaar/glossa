package app

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/felixgeelhaar/glossa/platform/internal/context/domain"
	"github.com/felixgeelhaar/glossa/platform/internal/identity/authz"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/problem"
)

// Service implements Context's use cases.
type Service struct {
	tx        Transactor
	catalog   Catalog
	sweeper   Sweeper
	retention domain.RetentionPolicy
	limiter   Limiter
	metrics   Metrics
	objects   Objects
	images    ImageNormalizer
	// deleteBatch bounds the object-store deletes one purge issues at a
	// time.
	deleteBatch int
	tracer      trace.Tracer
	logger      *slog.Logger
	now         func() time.Time
}

// tracerName names Context's spans' instrumentation scope.
const tracerName = "github.com/felixgeelhaar/glossa/platform/internal/context"

// WithTracerProvider makes Context trace a CI upload end to end
// (RFC 0004 §11: one trace per upload, ingest → events). The events it
// publishes carry the span's context, so the outbox's deliveries join
// the same trace. Without one, nothing is traced.
func WithTracerProvider(tp trace.TracerProvider) Option {
	return func(s *Service) {
		if tp != nil {
			s.tracer = tp.Tracer(tracerName)
		}
	}
}

// span starts a child span of ctx named name, and returns it with the
// function that ends it: end(err) records the error and finishes.
func (s *Service) span(ctx context.Context, name string, attrs ...attribute.KeyValue) (context.Context, func(*error)) {
	ctx, sp := s.tracer.Start(ctx, name, trace.WithSpanKind(trace.SpanKindInternal), trace.WithAttributes(attrs...))
	return ctx, func(err *error) {
		if err != nil && *err != nil {
			sp.RecordError(*err)
			sp.SetStatus(codes.Error, (*err).Error())
		}
		sp.End()
	}
}

// DefaultDeleteBatch is how many images a purge deletes from object
// storage at a time without WithDeleteBatch.
const DefaultDeleteBatch = 100

// WithDeleteBatch bounds the object-store deletes one purge issues at a
// time. A batch of 0 or less keeps the default.
func WithDeleteBatch(n int) Option {
	return func(s *Service) {
		if n > 0 {
			s.deleteBatch = n
		}
	}
}

// WithLimiter rate-limits uploads per tenant (RFC 0004 §10); without
// one they aren't limited.
func WithLimiter(l Limiter) Option { return func(s *Service) { s.limiter = l } }

// WithImages stores capture images in objects after normalizer has
// validated and re-encoded them (RFC 0004 §3.3). Without it, capture
// uploads and image reads fail, and purges leave images in place.
func WithImages(objects Objects, normalizer ImageNormalizer) Option {
	return func(s *Service) { s.objects, s.images = objects, normalizer }
}

// WithMetrics records ingests and coverage (NoMetrics by default).
func WithMetrics(m Metrics) Option { return func(s *Service) { s.metrics = m } }

// Option configures a Service.
type Option func(*Service)

// WithClock replaces time.Now (tests).
func WithClock(now func() time.Time) Option { return func(s *Service) { s.now = now } }

// WithLogger sets the logger for background work.
func WithLogger(l *slog.Logger) Option { return func(s *Service) { s.logger = l } }

// WithRetention replaces domain.DefaultRetention.
func WithRetention(p domain.RetentionPolicy) Option { return func(s *Service) { s.retention = p } }

// WithSweeper enables Purge across tenants.
func WithSweeper(sw Sweeper) Option { return func(s *Service) { s.sweeper = sw } }

// New returns the service.
func New(tx Transactor, catalog Catalog, opts ...Option) *Service {
	s := &Service{
		tx: tx, catalog: catalog, retention: domain.DefaultRetention, metrics: NoMetrics{}, logger: slog.New(slog.DiscardHandler),
		deleteBatch: DefaultDeleteBatch, tracer: noop.NewTracerProvider().Tracer(tracerName),
		now: func() time.Time { return time.Now().UTC() },
	}
	for _, o := range opts {
		o(s)
	}
	return s
}

// actor returns the acting principal after checking perm.
func actor(ctx context.Context, perm authz.Permission) (string, error) {
	if err := authz.Require(ctx, perm); err != nil {
		return "", err
	}
	p, _ := authz.From(ctx)
	return p.Actor.String(), nil
}

func invalidPageToken() error {
	return problem.New(http.StatusBadRequest, "invalid_page_token", "page_token is not one this list issued")
}

// parseView reads a branch view: empty is the default branch's.
func parseView(branch string) (domain.Branch, error) {
	if branch == "" {
		return "", nil
	}
	return domain.ParseBranch(branch)
}
