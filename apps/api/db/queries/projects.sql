-- name: CreateProject :one
INSERT INTO projects (tenant_id, slug, name, default_locale, api_key_hash)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, tenant_id, slug, name, default_locale, created_at;

-- name: GetProjectBySlug :one
SELECT id, tenant_id, slug, name, default_locale, api_key_hash, created_at
FROM projects
WHERE tenant_id = $1 AND slug = $2;

-- name: GetProjectByAPIKeyHash :one
-- Lookup used by the API-key auth middleware. The hash is SHA-256 of
-- the raw key; the comparison is timing-safe at the driver level
-- (Postgres byte equality on a fixed-length BYTEA).
SELECT id, tenant_id, slug, name, default_locale, created_at
FROM projects
WHERE api_key_hash = $1;

-- name: ListProjectsForTenant :many
SELECT id, slug, name, default_locale, created_at
FROM projects
WHERE tenant_id = $1
ORDER BY created_at DESC;

-- name: RotateProjectAPIKey :exec
UPDATE projects SET api_key_hash = $2 WHERE id = $1;
