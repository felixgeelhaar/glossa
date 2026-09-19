-- Tenant scope (db.TenantTx): RLS limits every statement to the current
-- tenant.
--
-- Scope filter used by the lookups: all_projects reaches every unit of
-- the tenant; otherwise tenant-wide units and the given project's.

-- ── derivation ─────────────────────────────────────────────────────

-- name: EnsureDerivation :exec
-- Creates the translation's bookkeeping row so LockDerivation always
-- has a row to lock; a concurrent insert waits for the first to commit.
INSERT INTO knowledge_tm_derivations (translation_id, tenant_id, project_id, revision, updated_at)
VALUES (sqlc.arg(translation_id), app_current_tenant(), sqlc.arg(project_id), 0, sqlc.arg(updated_at))
ON CONFLICT (translation_id) DO NOTHING;

-- name: LockDerivation :one
SELECT revision FROM knowledge_tm_derivations WHERE translation_id = sqlc.arg(translation_id) FOR UPDATE;

-- name: SetDerivation :exec
UPDATE knowledge_tm_derivations SET revision = sqlc.arg(revision), updated_at = sqlc.arg(updated_at)
WHERE translation_id = sqlc.arg(translation_id);

-- name: LockActiveUnitOfTranslation :one
SELECT * FROM knowledge_tm_units
WHERE translation_id = sqlc.arg(translation_id) AND retired_at IS NULL
FOR UPDATE;

-- ── units ──────────────────────────────────────────────────────────

-- name: InsertTMUnit :exec
INSERT INTO knowledge_tm_units (id, tenant_id, project_id, origin, translation_id, translation_revision, message_id,
                                message_key, namespace, source_locale, target_locale, source_mf2, target_mf2,
                                target_model, source_normalized, target_normalized, source_hash, signature,
                                source_vars, hit_count, last_hit_at, created_by, created_at, updated_at)
VALUES (sqlc.arg(id), app_current_tenant(), sqlc.narg(project_id), sqlc.arg(origin), sqlc.narg(translation_id),
        sqlc.narg(translation_revision), sqlc.narg(message_id), sqlc.arg(message_key), sqlc.arg(namespace),
        sqlc.arg(source_locale), sqlc.arg(target_locale), sqlc.arg(source_mf2), sqlc.arg(target_mf2),
        sqlc.arg(target_model), sqlc.arg(source_normalized), sqlc.arg(target_normalized), sqlc.arg(source_hash),
        sqlc.arg(signature), sqlc.arg(source_vars), 0, NULL, sqlc.arg(created_by), sqlc.arg(created_at),
        sqlc.arg(updated_at));

-- name: TouchTMUnit :exec
UPDATE knowledge_tm_units
SET translation_revision = sqlc.arg(translation_revision), updated_at = sqlc.arg(updated_at)
WHERE id = sqlc.arg(id);

-- name: RetireTMUnit :execrows
UPDATE knowledge_tm_units
SET retired_at = sqlc.arg(retired_at), retired_reason = sqlc.arg(retired_reason), retired_by = sqlc.arg(retired_by),
    updated_at = sqlc.arg(retired_at)
WHERE id = sqlc.arg(id) AND retired_at IS NULL;

-- name: GetTMUnit :one
SELECT * FROM knowledge_tm_units WHERE id = sqlc.arg(id);

-- name: LockTMUnit :one
SELECT * FROM knowledge_tm_units WHERE id = sqlc.arg(id) FOR UPDATE;

-- name: ListTMUnits :many
-- Units in id order after the cursor. state: active, retired or all.
SELECT * FROM knowledge_tm_units
WHERE id > sqlc.arg(after)
  AND (sqlc.narg(source_locale)::text IS NULL OR source_locale = sqlc.narg(source_locale))
  AND (sqlc.narg(target_locale)::text IS NULL OR target_locale = sqlc.narg(target_locale))
  AND (sqlc.narg(project_id)::uuid IS NULL OR project_id = sqlc.narg(project_id))
  AND (sqlc.narg(translation_id)::uuid IS NULL OR translation_id = sqlc.narg(translation_id))
  AND (sqlc.arg(state)::text = 'all'
       OR (sqlc.arg(state)::text = 'active' AND retired_at IS NULL)
       OR (sqlc.arg(state)::text = 'retired' AND retired_at IS NOT NULL))
ORDER BY id
LIMIT sqlc.arg(max_rows);

-- name: CountTMHits :exec
UPDATE knowledge_tm_units SET hit_count = hit_count + 1, last_hit_at = sqlc.arg(hit_at)
WHERE id = ANY (sqlc.arg(ids)::uuid[]);

-- name: DeleteProjectTM :exec
DELETE FROM knowledge_tm_units WHERE project_id = sqlc.arg(project_id);

-- name: DeleteProjectDerivations :exec
DELETE FROM knowledge_tm_derivations WHERE project_id = sqlc.arg(project_id);

-- ── lookups ────────────────────────────────────────────────────────

-- name: ExactTMMatches :many
-- Same normalized text and placeholder signature in the locale pair,
-- the query's project first, then the most recently confirmed.
SELECT * FROM knowledge_tm_units
WHERE retired_at IS NULL
  AND source_locale = sqlc.arg(source_locale) AND target_locale = sqlc.arg(target_locale)
  AND source_hash = sqlc.arg(source_hash) AND signature = sqlc.arg(signature)
  AND (sqlc.arg(all_projects)::boolean OR project_id IS NULL OR project_id = sqlc.narg(project_id))
ORDER BY (project_id IS NOT DISTINCT FROM sqlc.narg(project_id)) DESC, updated_at DESC, id
LIMIT sqlc.arg(max_rows);

-- name: SetSimilarityThreshold :exec
-- The % operator (and so the trigram index) filters by this, for the
-- rest of the transaction.
SELECT set_config('pg_trgm.similarity_threshold', sqlc.arg(threshold)::text, true);

-- name: FuzzyTMMatches :many
-- Trigram-similar normalized text in the locale pair, most similar
-- first.
SELECT sqlc.embed(knowledge_tm_units),
       similarity(knowledge_tm_units.source_normalized, sqlc.arg(query)::text)::float8 AS similarity
FROM knowledge_tm_units
WHERE retired_at IS NULL
  AND source_locale = sqlc.arg(source_locale) AND target_locale = sqlc.arg(target_locale)
  AND (sqlc.arg(all_projects)::boolean OR project_id IS NULL OR project_id = sqlc.narg(project_id))
  AND source_normalized % sqlc.arg(query)::text
ORDER BY similarity DESC, (project_id IS NOT DISTINCT FROM sqlc.narg(project_id)) DESC, updated_at DESC, id
LIMIT sqlc.arg(max_rows);

-- name: ConcordanceSource :many
-- Active units whose source contains the pattern (ILIKE, trigram
-- indexed), closest first.
SELECT sqlc.embed(knowledge_tm_units),
       word_similarity(sqlc.arg(query)::text, knowledge_tm_units.source_normalized)::float8 AS similarity
FROM knowledge_tm_units
WHERE retired_at IS NULL
  AND (sqlc.narg(source_locale)::text IS NULL OR source_locale = sqlc.narg(source_locale))
  AND (sqlc.narg(target_locale)::text IS NULL OR target_locale = sqlc.narg(target_locale))
  AND (sqlc.arg(all_projects)::boolean OR project_id IS NULL OR project_id = sqlc.narg(project_id))
  AND source_normalized ILIKE sqlc.arg(pattern)::text
ORDER BY similarity DESC, updated_at DESC, id
LIMIT sqlc.arg(max_rows);

-- name: ConcordanceTarget :many
SELECT sqlc.embed(knowledge_tm_units),
       word_similarity(sqlc.arg(query)::text, knowledge_tm_units.target_normalized)::float8 AS similarity
FROM knowledge_tm_units
WHERE retired_at IS NULL
  AND (sqlc.narg(source_locale)::text IS NULL OR source_locale = sqlc.narg(source_locale))
  AND (sqlc.narg(target_locale)::text IS NULL OR target_locale = sqlc.narg(target_locale))
  AND (sqlc.arg(all_projects)::boolean OR project_id IS NULL OR project_id = sqlc.narg(project_id))
  AND target_normalized ILIKE sqlc.arg(pattern)::text
ORDER BY similarity DESC, updated_at DESC, id
LIMIT sqlc.arg(max_rows);
