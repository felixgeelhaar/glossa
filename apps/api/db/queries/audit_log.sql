-- Audit log — records every translation mutation. Append-only;
-- reads are tenant-scoped via RLS just like the rest of the schema.

-- name: AppendAuditEntry :exec
INSERT INTO audit_log (tenant_id, translation_id, before_value, after_value, changed_by, actor_kind, actor_label)
VALUES ($1, $2, $3, $4, $5, $6, $7);

-- name: ListAuditForTenant :many
SELECT id, tenant_id, translation_id, before_value, after_value, changed_by, actor_kind, actor_label, changed_at
FROM audit_log
WHERE tenant_id = $1
ORDER BY changed_at DESC
LIMIT $2 OFFSET $3;
