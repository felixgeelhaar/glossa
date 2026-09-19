package app

import (
	"context"
	"log/slog"
	"time"

	"github.com/felixgeelhaar/glossa/platform/internal/context/domain"
	"github.com/felixgeelhaar/glossa/platform/internal/identity/authz"
)

// Service implements Context's use cases.
type Service struct {
	tx        Transactor
	catalog   Catalog
	sweeper   Sweeper
	retention domain.RetentionPolicy
	logger    *slog.Logger
	now       func() time.Time
}

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
		tx: tx, catalog: catalog, retention: domain.DefaultRetention, logger: slog.New(slog.DiscardHandler),
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

// parseView reads a branch view: empty is the default branch's.
func parseView(branch string) (domain.Branch, error) {
	if branch == "" {
		return "", nil
	}
	return domain.ParseBranch(branch)
}
