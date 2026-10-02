package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"

	"github.com/felixgeelhaar/glossa/platform/internal/audit/domain"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/observability"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/outbox"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/tenancy"
)

// Subscriber is the outbox subscriber name the projection runs under.
// It is persisted with each event's delivered_to, so it never changes.
const Subscriber = "audit.record"

// Service implements the Audit context's use cases.
type Service struct {
	store   Store
	history History
	logger  *slog.Logger
	keys    *domain.KeySet
}

// Option configures the service.
type Option func(*Service)

// WithHistory gives the service the outbox history the backfill reads.
func WithHistory(h History) Option { return func(s *Service) { s.history = h } }

// WithExportKeys gives the service the deployment's audit key set: the
// key exports are signed with and the public keys they verify with.
func WithExportKeys(k *domain.KeySet) Option { return func(s *Service) { s.keys = k } }

// ExportKeys is the audit key set, nil when none is configured. The
// export jobs (RFC 0006 wave 5) sign with its active key and serve its
// public keys as a glossa.audit.keys/1 document at
// /.well-known/glossa-audit-keys.json.
func (s *Service) ExportKeys() *domain.KeySet { return s.keys }

// WithLogger sets the logger. Audit logs ids, counts and actions, never
// a summary's values.
func WithLogger(l *slog.Logger) Option {
	return func(s *Service) {
		if l != nil {
			s.logger = l
		}
	}
}

// New returns the service on store.
func New(store Store, opts ...Option) *Service {
	s := &Service{store: store, logger: slog.New(slog.DiscardHandler)}
	for _, o := range opts {
		o(s)
	}
	return s
}

var _ Recorder = (*Service)(nil)

// Subscribe subscribes the projection to every event type domain.
// Projections names. The registry test in domain is what makes that
// every event type there is.
func (s *Service) Subscribe(reg *outbox.Registry) error {
	for _, typ := range domain.ProjectedTypes() {
		if err := reg.Subscribe(typ, Subscriber, outbox.HandlerFunc(s.HandleEvent)); err != nil {
			return err
		}
	}
	return nil
}

// HandleEvent projects one delivered event into the tenant's chain. It
// is idempotent on the event id: a redelivery appends nothing.
func (s *Service) HandleEvent(ctx context.Context, d outbox.Delivery) error {
	draft, err := domain.FromEvent(d)
	if err != nil {
		return outbox.Permanent(err)
	}
	if _, err := s.Append(ctx, draft); err != nil {
		if errors.Is(err, domain.ErrInvalidEntry) {
			return outbox.Permanent(err)
		}
		return err
	}
	return nil
}

// RecordSignIn implements Recorder.
func (s *Service) RecordSignIn(ctx context.Context, in domain.SignIn) error {
	if in.RequestID == "" {
		in.RequestID, _ = observability.RequestIDFromContext(ctx)
	}
	_, err := s.Append(ctx, in.Draft())
	return err
}

// RecordToolCall implements Recorder.
func (s *Service) RecordToolCall(ctx context.Context, c domain.ToolCall) error {
	if c.RequestID == "" {
		c.RequestID, _ = observability.RequestIDFromContext(ctx)
	}
	_, err := s.Append(ctx, c.Draft())
	return err
}

// Append records drafts, in order, at the end of the chain of ctx's
// tenant, skipping any whose event id is already recorded. It returns
// the entries it appended. Every draft is checked before anything is
// written, and all are written in one transaction or none is.
func (s *Service) Append(ctx context.Context, drafts ...domain.Draft) ([]domain.Entry, error) {
	if len(drafts) == 0 {
		return nil, nil
	}
	ids := make([]uuid.UUID, 0, len(drafts))
	for _, d := range drafts {
		if err := d.Validate(); err != nil {
			return nil, err
		}
		ids = append(ids, d.EventID)
	}
	var appended []domain.Entry
	err := s.store.InChain(ctx, func(ctx context.Context, c Chain) error {
		appended = appended[:0]
		head, err := c.Head(ctx)
		if err != nil {
			return err
		}
		recorded, err := c.Recorded(ctx, ids)
		if err != nil {
			return err
		}
		for _, d := range drafts {
			if recorded[d.EventID] {
				continue
			}
			e, err := domain.Append(c.Tenant(), head, d)
			if err != nil {
				return err
			}
			if err := c.Insert(ctx, e); err != nil {
				return err
			}
			recorded[d.EventID] = true // a duplicate within one batch
			head = e.Head()
			appended = append(appended, e)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("audit: append: %w", err)
	}
	return appended, nil
}

// VerifyReport is the result of verifying a tenant's stored chain.
type VerifyReport struct {
	Entries int64
	// Head is the last entry verified.
	Head domain.Head
}

// verifyPage is how many entries Verify reads at a time.
const verifyPage = 1000

// Verify recomputes the whole chain of ctx's tenant from its first
// entry. It returns a *domain.ChainError at the first entry that does
// not follow, and the count verified up to there.
func (s *Service) Verify(ctx context.Context) (VerifyReport, error) {
	tenant, ok := tenancy.FromContext(ctx)
	if !ok {
		return VerifyReport{}, errors.New("audit: verify needs a tenant")
	}
	v := domain.NewVerifier(tenant.UUID(), domain.Head{})
	var after int64
	for {
		page, err := s.store.Entries(ctx, after, verifyPage)
		if err != nil {
			return VerifyReport{}, fmt.Errorf("audit: read the chain: %w", err)
		}
		for _, e := range page {
			if err := v.Next(e); err != nil {
				return VerifyReport{Entries: v.Count(), Head: v.Head()}, err
			}
			after = e.Sequence
		}
		if len(page) < verifyPage {
			return VerifyReport{Entries: v.Count(), Head: v.Head()}, nil
		}
	}
}
