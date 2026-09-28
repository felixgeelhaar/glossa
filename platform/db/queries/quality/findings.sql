-- name: LatestRun :one
-- The latest run of a ref, or of the project when ref is empty: what
-- every surface reads (migration 0027, quality_check_runs_ref).
SELECT id, ref, trigger, policy_version, layers, errors, warnings, waived,
       conclusion, created_by, started_at, completed_at
FROM quality_check_runs
WHERE project_id = $1
  AND (sqlc.arg(ref)::text = '' OR ref = sqlc.arg(ref)::text)
  AND (NOT sqlc.arg(completed_only)::boolean OR completed_at IS NOT NULL)
ORDER BY started_at DESC, id DESC
LIMIT 1;

-- name: RunFindings :many
-- One run's findings, filtered and keyset-paginated on the finding id.
SELECT id, fingerprint, layer, code, severity, message_id, message_key, locale,
       namespace, file, line, col, route, component, capture_id, region,
       span_side, span_start, span_end, explanation, subject, detail,
       source_revision, waiver_id
FROM quality_findings
WHERE run_id = $1
  AND (sqlc.arg(layer)::text = '' OR layer = sqlc.arg(layer)::text)
  AND (sqlc.arg(locale)::text = '' OR locale = sqlc.arg(locale)::text)
  AND (sqlc.arg(severity)::text = '' OR severity = sqlc.arg(severity)::text)
  AND (sqlc.arg(message_key)::text = '' OR message_key = sqlc.arg(message_key)::text)
  AND (NOT sqlc.arg(waived_only)::boolean OR severity = 'waived')
  AND (sqlc.arg(after)::uuid = '00000000-0000-0000-0000-000000000000'::uuid OR id > sqlc.arg(after)::uuid)
ORDER BY id
LIMIT sqlc.arg(row_limit)::integer;
