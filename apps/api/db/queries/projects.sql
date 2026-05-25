-- name: CreateProject :one
INSERT INTO projects (id, tenant_id, slug, name, default_locale)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, tenant_id, slug, name, default_locale, created_at;

-- name: GetProjectBySlug :one
SELECT id, tenant_id, slug, name, default_locale, created_at
FROM projects
WHERE tenant_id = $1 AND slug = $2;

-- name: ListProjectsForTenant :many
SELECT id, slug, name, default_locale, created_at
FROM projects
WHERE tenant_id = $1
ORDER BY created_at DESC;
