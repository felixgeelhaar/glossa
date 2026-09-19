-- Tenant scope (db.TenantTx): RLS limits every statement to the current
-- tenant.

-- ── message projection ─────────────────────────────────────────────

-- name: LockMessageState :one
SELECT * FROM localization_messages WHERE message_id = sqlc.arg(message_id) FOR UPDATE;

-- name: UpsertMessageState :execrows
-- Keeps the snapshot with the highest version; an older or repeated
-- event changes nothing.
INSERT INTO localization_messages (tenant_id, message_id, project_id, key, namespace, state,
                                   source_revision, version, updated_at)
VALUES (app_current_tenant(), sqlc.arg(message_id), sqlc.arg(project_id), sqlc.arg(key), sqlc.arg(namespace),
        sqlc.arg(state), sqlc.arg(source_revision), sqlc.arg(version), sqlc.arg(updated_at))
ON CONFLICT (message_id) DO UPDATE
SET key = EXCLUDED.key, namespace = EXCLUDED.namespace, state = EXCLUDED.state,
    source_revision = GREATEST(localization_messages.source_revision, EXCLUDED.source_revision),
    version = EXCLUDED.version, updated_at = EXCLUDED.updated_at
WHERE localization_messages.version < EXCLUDED.version;

-- name: DeleteProjectMessages :exec
DELETE FROM localization_messages WHERE project_id = sqlc.arg(project_id);

-- name: MessagesMissingIn :many
-- Messages with no translation in a locale, in key order.
SELECT m.message_id, m.key FROM localization_messages m
WHERE m.project_id = sqlc.arg(project_id)
  AND m.key > sqlc.arg(after)
  AND (sqlc.narg(namespace)::text IS NULL OR m.namespace = sqlc.narg(namespace))
  AND (sqlc.narg(state)::text IS NULL OR m.state = sqlc.narg(state))
  AND (sqlc.narg(key_like)::text IS NULL OR m.key LIKE sqlc.narg(key_like))
  AND NOT EXISTS (SELECT FROM localization_translations t
                  WHERE t.message_id = m.message_id AND t.locale = sqlc.arg(locale))
ORDER BY m.key
LIMIT sqlc.arg(max_rows);

-- name: MessagesOutdatedIn :many
-- Messages whose translation in a locale was made against an older
-- source revision, in key order.
SELECT m.message_id, m.key FROM localization_messages m
WHERE m.project_id = sqlc.arg(project_id)
  AND m.key > sqlc.arg(after)
  AND (sqlc.narg(namespace)::text IS NULL OR m.namespace = sqlc.narg(namespace))
  AND (sqlc.narg(state)::text IS NULL OR m.state = sqlc.narg(state))
  AND (sqlc.narg(key_like)::text IS NULL OR m.key LIKE sqlc.narg(key_like))
  AND EXISTS (SELECT FROM localization_translations t
              WHERE t.message_id = m.message_id AND t.locale = sqlc.arg(locale)
                AND t.source_revision < m.source_revision)
ORDER BY m.key
LIMIT sqlc.arg(max_rows);

-- ── translations ───────────────────────────────────────────────────

-- name: InsertTranslation :exec
INSERT INTO localization_translations (id, tenant_id, project_id, message_id, locale, syntax, text, model,
                                       state, origin, author, source_revision, warnings, revision,
                                       created_at, updated_at)
VALUES (sqlc.arg(id), app_current_tenant(), sqlc.arg(project_id), sqlc.arg(message_id), sqlc.arg(locale),
        sqlc.arg(syntax), sqlc.arg(text), sqlc.arg(model), sqlc.arg(state), sqlc.arg(origin), sqlc.arg(author),
        sqlc.arg(source_revision), sqlc.arg(warnings), sqlc.arg(revision), sqlc.arg(created_at),
        sqlc.arg(updated_at));

-- name: UpdateTranslation :execrows
UPDATE localization_translations
SET syntax = sqlc.arg(syntax), text = sqlc.arg(text), model = sqlc.arg(model), state = sqlc.arg(state),
    origin = sqlc.arg(origin), author = sqlc.arg(author), source_revision = sqlc.arg(source_revision),
    warnings = sqlc.arg(warnings), revision = sqlc.arg(revision), updated_at = sqlc.arg(updated_at)
WHERE id = sqlc.arg(id) AND revision = sqlc.arg(expected_revision);

-- name: GetTranslation :one
-- With the message's current source revision, from which "outdated" is
-- derived (0 while Localization hasn't seen the message).
SELECT t.*, coalesce(m.source_revision, 0)::integer AS current_source_revision
FROM localization_translations t
LEFT JOIN localization_messages m ON m.message_id = t.message_id
WHERE t.message_id = sqlc.arg(message_id) AND t.locale = sqlc.arg(locale);

-- name: LockTranslation :one
SELECT * FROM localization_translations
WHERE message_id = sqlc.arg(message_id) AND locale = sqlc.arg(locale)
FOR UPDATE;

-- name: ListTranslationsOfMessage :many
SELECT t.*, coalesce(m.source_revision, 0)::integer AS current_source_revision
FROM localization_translations t
LEFT JOIN localization_messages m ON m.message_id = t.message_id
WHERE t.message_id = sqlc.arg(message_id) AND t.locale > sqlc.arg(after)
ORDER BY t.locale
LIMIT sqlc.arg(max_rows);

-- name: NewlyOutdatedTranslations :many
-- Translations that were current at old_revision and are behind
-- new_revision.
SELECT * FROM localization_translations
WHERE message_id = sqlc.arg(message_id)
  AND source_revision >= sqlc.arg(old_revision) AND source_revision < sqlc.arg(new_revision)
ORDER BY locale;

-- name: SnapshotTranslations :many
-- A project's translations in the given review states, for a release.
SELECT t.*, coalesce(m.source_revision, 0)::integer AS current_source_revision
FROM localization_translations t
LEFT JOIN localization_messages m ON m.message_id = t.message_id
WHERE t.project_id = sqlc.arg(project_id) AND t.state = ANY (sqlc.arg(states)::text[])
ORDER BY t.locale, t.message_id;

-- name: PageProjectTranslations :many
-- A project's translations in some locales, across messages, in
-- (key, message_id, locale) order after the cursor. Driven by
-- localization_messages_project_order; translations of locales the
-- project no longer has are left out.
SELECT t.*, m.key, m.namespace, m.state AS message_state, m.source_revision AS current_source_revision
FROM localization_messages m
JOIN localization_translations t ON t.message_id = m.message_id
JOIN localization_locales l ON l.project_id = t.project_id AND l.code = t.locale
WHERE m.project_id = sqlc.arg(project_id)
  AND t.project_id = sqlc.arg(project_id)
  AND t.locale = ANY (sqlc.arg(locales)::text[])
  -- The first comparison is implied by the second; it is the one the
  -- index can seek to, so a deep page doesn't rescan earlier keys.
  AND (m.key, m.message_id) >= (sqlc.arg(after_key)::text, sqlc.arg(after_message)::uuid)
  AND (m.key, m.message_id, t.locale) > (sqlc.arg(after_key)::text, sqlc.arg(after_message)::uuid,
                                         sqlc.arg(after_locale)::text)
  AND (sqlc.narg(states)::text[] IS NULL OR t.state = ANY (sqlc.narg(states)::text[]))
  AND (sqlc.narg(outdated)::boolean IS NULL OR (t.source_revision < m.source_revision) = sqlc.narg(outdated))
  AND (sqlc.narg(namespace)::text IS NULL OR m.namespace = sqlc.narg(namespace))
  AND (sqlc.narg(message_state)::text IS NULL OR m.state = sqlc.narg(message_state))
  AND (sqlc.narg(key_like)::text IS NULL OR m.key LIKE sqlc.narg(key_like))
ORDER BY m.key, m.message_id, t.locale
LIMIT sqlc.arg(max_rows);

-- name: DeleteProjectTranslations :exec
DELETE FROM localization_translations WHERE project_id = sqlc.arg(project_id);

-- name: InsertTranslationRevision :exec
INSERT INTO localization_translation_revisions (tenant_id, translation_id, revision, kind, syntax, text, model,
                                                state, origin, origin_detail, author, source_revision,
                                                findings, created_at)
VALUES (app_current_tenant(), sqlc.arg(translation_id), sqlc.arg(revision), sqlc.arg(kind), sqlc.arg(syntax),
        sqlc.arg(text), sqlc.arg(model), sqlc.arg(state), sqlc.arg(origin), sqlc.arg(origin_detail),
        sqlc.arg(author), sqlc.arg(source_revision), sqlc.arg(findings), sqlc.arg(created_at));

-- name: ListTranslationRevisions :many
-- Newest first; before is exclusive.
SELECT * FROM localization_translation_revisions
WHERE translation_id = sqlc.arg(translation_id) AND revision < sqlc.arg(before)
ORDER BY revision DESC
LIMIT sqlc.arg(max_rows);
