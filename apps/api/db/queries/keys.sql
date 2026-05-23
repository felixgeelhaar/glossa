-- name: UpsertKey :one
-- Idempotent insert used by the CLI scanner. New keys come in;
-- existing keys keep their description but advance first_seen_at
-- conceptually still equals the original. Description is updated on
-- match (so a rename in source surfaces in the admin).
INSERT INTO keys (project_id, key, description)
VALUES ($1, $2, $3)
ON CONFLICT (project_id, key) DO UPDATE
  SET description = COALESCE(EXCLUDED.description, keys.description)
RETURNING id, project_id, key, description, first_seen_at;

-- name: ListKeysForProject :many
SELECT id, key, description, first_seen_at
FROM keys
WHERE project_id = $1
ORDER BY key ASC;

-- name: GetKey :one
SELECT id, project_id, key, description, first_seen_at
FROM keys
WHERE project_id = $1 AND key = $2;
