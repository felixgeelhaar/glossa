-- Tenant scope (db.TenantTx): RLS limits every statement to the current
-- tenant.

-- name: InsertUsages :exec
-- A batch of a build's usages, by position in the document. uuid.Nil
-- is an unknown key's message ID, 0 an unknown column.
INSERT INTO context_usages (build_id, position, tenant_id, message_key, message_id, file, line, col, component, route, kind)
SELECT sqlc.arg(build_id), u.position, app_current_tenant(), u.message_key,
       NULLIF(u.message_id, '00000000-0000-0000-0000-000000000000'::uuid), u.file, u.line, NULLIF(u.col, 0),
       u.component, u.route, u.kind
FROM (SELECT unnest(sqlc.arg(positions)::int[]) AS position, unnest(sqlc.arg(keys)::text[]) AS message_key,
             unnest(sqlc.arg(message_ids)::uuid[]) AS message_id, unnest(sqlc.arg(files)::text[]) AS file,
             unnest(sqlc.arg(lines)::int[]) AS line, unnest(sqlc.arg(cols)::int[]) AS col,
             unnest(sqlc.arg(components)::text[]) AS component, unnest(sqlc.arg(routes)::text[]) AS route,
             unnest(sqlc.arg(kinds)::text[]) AS kind) AS u;

-- name: CountUnknownKeys :one
SELECT count(*) FROM context_usages WHERE build_id = sqlc.arg(build_id) AND message_id IS NULL;

-- name: ListMessageUsages :many
-- A message's usages in the given (current) builds: the default branch
-- first, then by application, file and position in the file.
SELECT u.build_id, u.position, u.message_key, u.message_id, u.file, u.line, u.col, u.component, u.route, u.kind,
       b.application_id, b.commit_sha, b.branch, b.on_default_branch, b.source
FROM context_usages u
JOIN context_builds b ON b.id = u.build_id
WHERE u.message_id = sqlc.arg(message_id) AND u.build_id = ANY(sqlc.arg(build_ids)::uuid[])
ORDER BY b.on_default_branch DESC, b.application_id, u.file, u.line, u.col NULLS FIRST, u.build_id, u.position
LIMIT sqlc.arg(max_rows);

-- name: ListUsedMessageIDs :many
-- The messages with a usage in the given (current) builds.
SELECT DISTINCT message_id::uuid AS message_id
FROM context_usages
WHERE build_id = ANY(sqlc.arg(build_ids)::uuid[]) AND message_id IS NOT NULL;
