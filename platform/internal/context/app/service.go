package app

import (
	"context"
	"log/slog"
	"net/http"
	"time"

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
	logger    *slog.Logger
	now       func() time.Time
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
