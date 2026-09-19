-- InsertOutboxEvent runs in the publisher's tenant-scoped transaction;
-- RLS rejects a tenant_id other than the transaction's tenant.

-- name: InsertOutboxEvent :exec
INSERT INTO outbox_events (
    id, tenant_id, event_type, aggregate_type, aggregate_id,
    payload, trace_context, occurred_at
) VALUES (
    sqlc.arg(id), sqlc.arg(tenant_id), sqlc.arg(event_type), sqlc.arg(aggregate_type),
    sqlc.arg(aggregate_id), sqlc.arg(payload), sqlc.arg(trace_context), sqlc.arg(occurred_at)
);

-- The remaining queries run in the relay's system scope (glossa_system).

-- ClaimOutboxEvents leases up to batch_size due events. SKIP LOCKED lets
-- replicas claim disjoint batches; the lease (available_at in the future)
-- keeps an event claimed after this short transaction commits, and a new
-- claim_token fences out a previous holder whose lease expired.
-- name: ClaimOutboxEvents :many
WITH due AS (
    SELECT o.id
    FROM outbox_events o
    WHERE o.status = 'pending' AND o.available_at <= now()
    ORDER BY o.available_at, o.id
    LIMIT sqlc.arg(batch_size)::int
    FOR UPDATE SKIP LOCKED
)
UPDATE outbox_events e
SET attempts     = e.attempts + 1,
    available_at = now() + make_interval(secs => sqlc.arg(lease_seconds)::float8),
    claim_token  = gen_random_uuid()
FROM due
WHERE e.id = due.id
RETURNING e.id, e.tenant_id, e.event_type, e.aggregate_type, e.aggregate_id,
          e.payload, e.trace_context, e.occurred_at, e.attempts,
          e.claim_token, e.delivered_to;

-- name: MarkOutboxEventDelivered :execrows
UPDATE outbox_events
SET status       = 'delivered',
    delivered_at = now(),
    delivered_to = sqlc.arg(delivered_to)::text[],
    last_error   = NULL,
    claim_token  = NULL
WHERE id = sqlc.arg(id) AND claim_token = sqlc.arg(claim_token)::uuid;

-- name: RescheduleOutboxEvent :execrows
UPDATE outbox_events
SET available_at = now() + make_interval(secs => sqlc.arg(delay_seconds)::float8),
    delivered_to = sqlc.arg(delivered_to)::text[],
    last_error   = sqlc.arg(last_error)::text,
    claim_token  = NULL
WHERE id = sqlc.arg(id) AND claim_token = sqlc.arg(claim_token)::uuid;

-- name: DeadLetterOutboxEvent :execrows
UPDATE outbox_events
SET status       = 'dead',
    dead_at      = now(),
    delivered_to = sqlc.arg(delivered_to)::text[],
    last_error   = sqlc.arg(last_error)::text,
    claim_token  = NULL
WHERE id = sqlc.arg(id) AND claim_token = sqlc.arg(claim_token)::uuid;

-- ReleaseOutboxEvent hands back a claim that was never attempted (the
-- dispatcher is shutting down or ran out of lease) without charging it
-- an attempt.
-- name: ReleaseOutboxEvent :execrows
UPDATE outbox_events
SET attempts     = greatest(attempts - 1, 0),
    available_at = now(),
    claim_token  = NULL
WHERE id = sqlc.arg(id) AND claim_token = sqlc.arg(claim_token)::uuid;
