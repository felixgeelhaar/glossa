package main

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	auditpg "github.com/felixgeelhaar/glossa/platform/internal/audit/adapters/postgres"
	auditapp "github.com/felixgeelhaar/glossa/platform/internal/audit/app"
	auditdomain "github.com/felixgeelhaar/glossa/platform/internal/audit/domain"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/config"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/db"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/outbox"
	releasedomain "github.com/felixgeelhaar/glossa/platform/internal/release/domain"
)

// newAudit wires the Audit context (RFC 0006 §6) and subscribes its
// projection to every event type. It is built before Identity and MCP,
// which record sign-ins and tool calls through it.
func newAudit(pool *pgxpool.Pool, events *outbox.Registry, keys *auditdomain.KeySet, logger *slog.Logger) (*auditapp.Service, error) {
	uow := db.NewUnitOfWork(pool)
	svc := auditapp.New(auditpg.NewStore(uow),
		auditapp.WithHistory(outbox.NewHistory(uow)), auditapp.WithExportKeys(keys), auditapp.WithLogger(logger))
	if err := svc.Subscribe(events); err != nil {
		return nil, err
	}
	return svc, nil
}

// newAuditKeys builds the audit export key set from GLOSSA_AUDIT_SIGNING_KEY
// and _RETIRED_KEYS (RFC 0006 §6.2). It runs before anything connects,
// so a key that can't be used stops the server at once. No key is nil:
// config.Load has already refused exports enabled without one, and
// unlike the release key there is no derived development key.
//
// The audit key must not be a release signing key: one key, one
// purpose. A deployment that pasted the same seed into both would let
// whoever holds the delivery plane's key sign audit evidence.
func newAuditKeys(cfg config.Config) (*auditdomain.KeySet, error) {
	if cfg.Audit.SigningKey.IsZero() {
		return nil, nil
	}
	pairs := config.KeyList(cfg.Audit.SigningKey.Reveal())
	active, err := auditdomain.ParseSigningKey(pairs[0][0], pairs[0][1])
	if err != nil {
		return nil, fmt.Errorf("GLOSSA_AUDIT_SIGNING_KEY: %w", err)
	}
	var retired []auditdomain.PublicKey
	for _, kv := range config.KeyList(cfg.Audit.RetiredKeys) {
		k, err := auditdomain.ParsePublicKey(kv[0], kv[1])
		if err != nil {
			return nil, fmt.Errorf("GLOSSA_AUDIT_RETIRED_KEYS: %w", err)
		}
		retired = append(retired, k)
	}
	pub := active.Public().Key
	for _, kv := range config.KeyList(cfg.Release.SigningKeys.Reveal()) {
		rk, err := releasedomain.ParseSigningKey(kv[0], kv[1])
		if err != nil {
			continue // newSigner reports it
		}
		if pub.Equal(rk.Key.Public()) {
			return nil, fmt.Errorf("GLOSSA_AUDIT_SIGNING_KEY: %w: it is the release signing key %q; audit exports need a key of their own",
				auditdomain.ErrInvalidKey, rk.ID)
		}
	}
	keys, err := auditdomain.NewKeySet(active, retired)
	if err != nil {
		return nil, fmt.Errorf("GLOSSA_AUDIT_SIGNING_KEY: %w", err)
	}
	return keys, nil
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
