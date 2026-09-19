-- Tenant scope (db.TenantTx): RLS limits every statement to the current
-- tenant.

-- name: InsertMessage :execrows
-- ON CONFLICT (id) makes a retried idempotent create a no-op; a key
-- clash still fails on the (project_id, key) constraint.
INSERT INTO catalog_messages (id, tenant_id, project_id, key, namespace, description, max_length,
                              state, source_syntax, source_text, source_model, arguments, markup,
                              source_revision, version, created_by, created_at, updated_at)
VALUES (sqlc.arg(id), app_current_tenant(), sqlc.arg(project_id), sqlc.arg(key), sqlc.arg(namespace),
        sqlc.arg(description), sqlc.narg(max_length), sqlc.arg(state), sqlc.arg(source_syntax),
        sqlc.arg(source_text), sqlc.arg(source_model), sqlc.arg(arguments), sqlc.arg(markup),
        sqlc.arg(source_revision), sqlc.arg(version), sqlc.arg(created_by), sqlc.arg(created_at),
        sqlc.arg(updated_at))
ON CONFLICT (id) DO NOTHING;

-- name: InsertSourceRevision :exec
INSERT INTO catalog_source_revisions (tenant_id, message_id, revision, syntax, text, model, author, created_at)
VALUES (app_current_tenant(), sqlc.arg(message_id), sqlc.arg(revision), sqlc.arg(syntax), sqlc.arg(text),
        sqlc.arg(model), sqlc.arg(author), sqlc.arg(created_at));

-- name: GetMessage :one
SELECT * FROM catalog_messages WHERE project_id = sqlc.arg(project_id) AND id = sqlc.arg(id);

-- name: GetMessageByKey :one
SELECT * FROM catalog_messages WHERE project_id = sqlc.arg(project_id) AND key = sqlc.arg(key);

-- name: LockMessageByKey :one
SELECT * FROM catalog_messages
WHERE project_id = sqlc.arg(project_id) AND key = sqlc.arg(key)
FOR UPDATE;

-- name: LockMessagesByKeys :many
-- One lock order (by key) for every bulk writer.
SELECT * FROM catalog_messages
WHERE project_id = sqlc.arg(project_id) AND key = ANY (sqlc.arg(keys)::text[])
ORDER BY key
FOR UPDATE;

-- name: GetMessagesByKeys :many
SELECT * FROM catalog_messages
WHERE project_id = sqlc.arg(project_id) AND key = ANY (sqlc.arg(keys)::text[])
ORDER BY key;

-- name: GetMessagesByIDs :many
SELECT * FROM catalog_messages
WHERE project_id = sqlc.arg(project_id) AND id = ANY (sqlc.arg(ids)::uuid[]);

-- name: ListMessages :many
-- Keyset pagination on key. key_like is a LIKE pattern with escaped
-- wildcards ('checkout.%'), so the text_pattern_ops index serves it.
SELECT * FROM catalog_messages
WHERE project_id = sqlc.arg(project_id)
  AND key > sqlc.arg(after)
  AND (sqlc.narg(namespace)::text IS NULL OR namespace = sqlc.narg(namespace))
  AND (sqlc.narg(state)::text IS NULL OR state = sqlc.narg(state))
  AND (sqlc.narg(key_like)::text IS NULL OR key LIKE sqlc.narg(key_like))
ORDER BY key
LIMIT sqlc.arg(max_rows);

-- name: ListNamespaces :many
-- A page of a project's namespaces after a name, with message counts by
-- state; catalog_messages_namespaces serves it from the index alone.
SELECT namespace,
       count(*) FILTER (WHERE state = 'active')::int AS active,
       count(*) FILTER (WHERE state = 'obsolete')::int AS obsolete
FROM catalog_messages
WHERE project_id = sqlc.arg(project_id) AND namespace > sqlc.arg(after)
GROUP BY namespace
ORDER BY namespace
LIMIT sqlc.arg(max_rows);

-- name: ListActiveMessages :many
-- Every active message of a project, for a release snapshot.
SELECT * FROM catalog_messages
WHERE project_id = sqlc.arg(project_id) AND state = 'active'
ORDER BY key;

-- name: UpdateMessage :execrows
UPDATE catalog_messages
SET key = sqlc.arg(key), namespace = sqlc.arg(namespace), description = sqlc.arg(description),
    max_length = sqlc.narg(max_length), state = sqlc.arg(state), source_syntax = sqlc.arg(source_syntax),
    source_text = sqlc.arg(source_text), source_model = sqlc.arg(source_model),
    arguments = sqlc.arg(arguments), markup = sqlc.arg(markup),
    source_revision = sqlc.arg(source_revision), version = sqlc.arg(version),
    updated_at = sqlc.arg(updated_at)
WHERE id = sqlc.arg(id) AND version = sqlc.arg(expected_version);

-- name: ListSourceRevisions :many
-- Newest first; before is exclusive.
SELECT * FROM catalog_source_revisions
WHERE message_id = sqlc.arg(message_id) AND revision < sqlc.arg(before)
ORDER BY revision DESC
LIMIT sqlc.arg(max_rows);

-- name: GetSourceRevision :one
SELECT * FROM catalog_source_revisions
WHERE message_id = sqlc.arg(message_id) AND revision = sqlc.arg(revision);
