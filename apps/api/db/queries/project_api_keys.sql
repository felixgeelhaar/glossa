-- name: CreateProjectAPIKey :one
INSERT INTO project_api_keys (project_id, hash, scope, label)
VALUES ($1, $2, $3, $4)
RETURNING id, project_id, scope, label, created_at, last_used_at, revoked_at;

-- name: ListProjectAPIKeys :many
SELECT id, project_id, scope, label, created_at, last_used_at, revoked_at
FROM project_api_keys
WHERE project_id = $1
ORDER BY created_at DESC;

-- name: GetProjectAPIKeyByHash :one
-- Hot path. Hits every API-key authed request. The partial unique
-- index on hash (WHERE revoked_at IS NULL) makes this O(1).
SELECT k.id, k.project_id, k.scope, k.label, k.created_at, k.last_used_at,
       p.tenant_id, p.slug AS project_slug, p.name AS project_name,
       p.default_locale
FROM project_api_keys k
JOIN projects p ON p.id = k.project_id
WHERE k.hash = $1 AND k.revoked_at IS NULL;

-- name: TouchProjectAPIKey :exec
UPDATE project_api_keys
SET last_used_at = NOW()
WHERE id = $1;

-- name: RevokeProjectAPIKey :exec
UPDATE project_api_keys
SET revoked_at = NOW()
WHERE id = $1;
