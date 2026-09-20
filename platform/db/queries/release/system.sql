-- System scope (db.SystemTx as glossa_system): migration 0015 opens
-- these columns to it, read-only, and nothing else. Each query finds
-- work across tenants; the work itself runs in each tenant's scope.

-- name: ListDuePublishRequests :many
-- System scope release.publisher.
SELECT tenant_id, project_id, environment FROM release_publish_requests
WHERE not_before <= sqlc.arg(now)
ORDER BY not_before, project_id, environment
LIMIT sqlc.arg(max_rows)::int;

-- name: ListStaleKeyIndexes :many
-- System scope release.key_index: active keys whose index object
-- predates the current format.
SELECT tenant_id, project_id, id FROM release_delivery_keys
WHERE revoked_at IS NULL AND index_version < sqlc.arg(index_version)
ORDER BY tenant_id, id
LIMIT sqlc.arg(max_rows)::int;
