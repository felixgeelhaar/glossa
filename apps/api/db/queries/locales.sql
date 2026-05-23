-- name: CreateLocale :one
INSERT INTO locales (project_id, code, label, enabled)
VALUES ($1, $2, $3, $4)
RETURNING id, project_id, code, label, enabled, created_at;

-- name: ListLocalesForProject :many
SELECT id, code, label, enabled, created_at
FROM locales
WHERE project_id = $1
ORDER BY code ASC;

-- name: SetLocaleEnabled :exec
UPDATE locales SET enabled = $2 WHERE id = $1;
