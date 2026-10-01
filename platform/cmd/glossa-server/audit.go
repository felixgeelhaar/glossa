package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	auditpg "github.com/felixgeelhaar/glossa/platform/internal/audit/adapters/postgres"
	auditapp "github.com/felixgeelhaar/glossa/platform/internal/audit/app"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/db"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/outbox"
)

// newAudit wires the Audit context (RFC 0006 §6) and subscribes its
// projection to every event type. It is built before Identity and MCP,
// which record sign-ins and tool calls through it.
func newAudit(pool *pgxpool.Pool, events *outbox.Registry, logger *slog.Logger) (*auditapp.Service, error) {
	uow := db.NewUnitOfWork(pool)
	svc := auditapp.New(auditpg.NewStore(uow),
		auditapp.WithHistory(outbox.NewHistory(uow)), auditapp.WithLogger(logger))
	if err := svc.Subscribe(events); err != nil {
		return nil, err
	}
	return svc, nil
}

// auditBackfillRetry is how long the audit backfill waits after a
// failure.
const auditBackfillRetry = time.Minute

// startAuditBackfill projects the outbox's history into the audit trail
// in the background, until it succeeds once (RFC 0006 §6.1, "History").
// Like the key index task it runs on every replica: it is idempotent,
// appends under the same per-tenant lock as live delivery, and skips a
// tenant whose history is already recorded, so after the first run a
// restart costs two counts per tenant.
func (a *app) startAuditBackfill(ctx context.Context) {
	if a.audit == nil {
		return
	}
	go func() {
		for {
			r, err := a.audit.Backfill(ctx)
			if err == nil {
				if r.Recorded > 0 {
					a.logger.InfoContext(ctx, "audit: outbox history backfilled",
						slog.Int("tenants", r.Tenants), slog.Int("events", r.Events), slog.Int("recorded", r.Recorded),
						slog.Int("actor_from_payload", r.ActorFromPayload), slog.Int("actor_unknown", r.Unknown),
						slog.Int("retired_types", r.Retired))
				}
				return
			}
			if ctx.Err() != nil {
				return
			}
			a.logger.WarnContext(ctx, "audit: outbox history not backfilled yet; retrying",
				slog.Any("error", err), slog.Int("recorded_so_far", r.Recorded), slog.Duration("retry_in", auditBackfillRetry))
			select {
			case <-ctx.Done():
				return
			case <-time.After(auditBackfillRetry):
			}
		}
	}()
}
