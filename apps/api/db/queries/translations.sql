-- name: UpsertTranslation :one
INSERT INTO translations (key_id, locale_id, value, status, updated_by)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (key_id, locale_id) DO UPDATE
  SET value = EXCLUDED.value,
      status = EXCLUDED.status,
      updated_by = EXCLUDED.updated_by,
      updated_at = NOW()
RETURNING id, key_id, locale_id, value, status, updated_by, updated_at;

-- name: ListBundle :many
-- Full message bundle for a (project, locale) — used by both the
-- runtime SDK and the CLI's pull command. Filters by approved when
-- the caller is a consumer; admin reads include all statuses.
SELECT k.key, t.value, t.status
FROM keys k
LEFT JOIN translations t
  ON t.key_id = k.id AND t.locale_id = $2
WHERE k.project_id = $1
ORDER BY k.key ASC;

-- name: GetTranslation :one
-- Pre-edit lookup for the audit log: read the current value at
-- (key, locale) so AuditEntry.BeforeValue isn't always empty.
SELECT id, key_id, locale_id, value, status, updated_by, updated_at
FROM translations
WHERE key_id = $1 AND locale_id = $2;
