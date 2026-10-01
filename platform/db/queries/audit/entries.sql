-- The Audit context's queries over audit_entries (migration 0045). All
-- run in the tenant's own scope; the table is INSERT and SELECT only.

-- LockAuditChain serializes appends to one tenant's chain for the rest
-- of the transaction. Two tenants never wait on each other (unless
-- their keys collide, which only costs a wait).
-- name: LockAuditChain :exec
SELECT pg_advisory_xact_lock(hashtextextended('glossa.audit_entries:' || sqlc.arg(tenant_id)::uuid::text, 0));

-- name: AuditHead :one
SELECT sequence, hash
FROM audit_entries
WHERE tenant_id = sqlc.arg(tenant_id)
ORDER BY sequence DESC
LIMIT 1;

-- AuditRecorded returns which of the given event ids already have an
-- entry: the idempotency check, made under the chain lock.
-- name: AuditRecorded :many
SELECT event_id
FROM audit_entries
WHERE tenant_id = sqlc.arg(tenant_id) AND event_id = ANY(sqlc.arg(event_ids)::uuid[]);

-- AuditOutboxCount counts the tenant's entries projected from the
-- outbox: when it equals the outbox's own count, the backfill has
-- nothing to do for the tenant.
-- name: AuditOutboxCount :one
SELECT count(*) FROM audit_entries WHERE tenant_id = sqlc.arg(tenant_id) AND source = 'outbox';

-- name: InsertAuditEntry :exec
INSERT INTO audit_entries (
    tenant_id, sequence, event_id, source, action, actor, occurred_at,
    aggregate_type, aggregate_id, project_id, locale, summary,
    request_id, trace_id, prev_hash, hash
) VALUES (
    sqlc.arg(tenant_id), sqlc.arg(sequence), sqlc.arg(event_id), sqlc.arg(source), sqlc.arg(action),
    sqlc.arg(actor), sqlc.arg(occurred_at), sqlc.arg(aggregate_type), sqlc.arg(aggregate_id),
    sqlc.narg(project_id), sqlc.narg(locale), sqlc.arg(summary), sqlc.narg(request_id), sqlc.narg(trace_id),
    sqlc.arg(prev_hash), sqlc.arg(hash)
);

-- AuditEntriesAfter reads the chain in order from after a sequence.
-- name: AuditEntriesAfter :many
SELECT tenant_id, sequence, event_id, source, action, actor, occurred_at,
       aggregate_type, aggregate_id, project_id, locale, summary,
       request_id, trace_id, prev_hash, hash
FROM audit_entries
WHERE tenant_id = sqlc.arg(tenant_id) AND sequence > sqlc.arg(after_sequence)
ORDER BY sequence
LIMIT sqlc.arg(page_size)::int;
