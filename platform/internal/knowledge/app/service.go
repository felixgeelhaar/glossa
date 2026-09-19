package app

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/felixgeelhaar/glossa/platform/internal/identity/authz"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/idempotency"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/problem"
)

// Service implements Knowledge's use cases.
type Service struct {
	tx           Transactor
	translations Translations
	projects     Projects
	logger       *slog.Logger
	now          func() time.Time
}

// Option configures a Service.
type Option func(*Service)

// WithClock replaces time.Now (tests).
func WithClock(now func() time.Time) Option { return func(s *Service) { s.now = now } }

// WithLogger sets the logger for background work.
func WithLogger(l *slog.Logger) Option { return func(s *Service) { s.logger = l } }

// New returns the service.
func New(tx Transactor, translations Translations, projects Projects, opts ...Option) *Service {
	s := &Service{
		tx: tx, translations: translations, projects: projects, logger: slog.New(slog.DiscardHandler),
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

// requireProject checks that a project scope names an existing project.
func (s *Service) requireProject(ctx context.Context, project *uuid.UUID) error {
	if project == nil {
		return nil
	}
	_, err := s.projects.Project(ctx, *project)
	return err
}

// idempotentID derives a create's ID from its Idempotency-Key, or a new
// time-ordered ID when there is none.
func idempotentID(ctx context.Context, operation, by, key string) (uuid.UUID, error) {
	if key == "" {
		return uuid.Must(uuid.NewV7()), nil
	}
	if err := idempotency.CheckKey(key); err != nil {
		return uuid.Nil, err
	}
	p, _ := authz.From(ctx)
	return idempotency.ID(operation, p.Tenant.String(), by, key), nil
}

// checkIfMatch applies optimistic concurrency: a change based on
// another version fails.
func checkIfMatch(version int, ifMatch *int) error {
	switch {
	case ifMatch == nil:
		return ErrPreconditionRequired
	case *ifMatch != version:
		return ErrPreconditionFailed
	}
	return nil
}

func invalidPageToken() error {
	return problem.New(http.StatusBadRequest, "invalid_page_token", "page_token is not one this list issued")
}

// afterUUID reads a page cursor holding an ID.
func afterUUID(s string) (uuid.UUID, error) {
	if s == "" {
		return uuid.Nil, nil
	}
	id, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil, invalidPageToken()
	}
	return id, nil
}

// beforeVersion reads a page cursor holding a version number.
func beforeVersion(s string) (int, error) {
	if s == "" {
		return int(^uint32(0) >> 1), nil
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 {
		return 0, invalidPageToken()
	}
	return n, nil
}
