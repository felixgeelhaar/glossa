-- Tenant scope (db.TenantTx): RLS limits every statement to the current
-- tenant.

-- name: InsertBuild :execrows
-- An upload is idempotent by (application, commit, source, digest): a
-- repeat inserts nothing and the caller reads the first.
INSERT INTO context_builds (id, tenant_id, project_id, application_id, commit_sha, branch, on_default_branch, source,
                            tool_name, tool_version, digest, usage_count, created_by, created_at)
VALUES (sqlc.arg(id), app_current_tenant(), sqlc.arg(project_id), sqlc.arg(application_id), sqlc.arg(commit_sha),
        sqlc.arg(branch), sqlc.arg(on_default_branch), sqlc.arg(source), sqlc.arg(tool_name), sqlc.arg(tool_version),
        sqlc.arg(digest), sqlc.arg(usage_count), sqlc.arg(created_by), sqlc.arg(created_at))
ON CONFLICT (tenant_id, application_id, commit_sha, source, digest) DO NOTHING;

-- name: GetBuildByUpload :one
SELECT * FROM context_builds
WHERE tenant_id = app_current_tenant() AND application_id = sqlc.arg(application_id)
  AND commit_sha = sqlc.arg(commit_sha) AND source = sqlc.arg(source) AND digest = sqlc.arg(digest);

-- name: GetBuild :one
SELECT * FROM context_builds WHERE id = sqlc.arg(id);

-- name: LockBuildCaptures :exec
-- Serializes the captures added to one build, so the per-build limit
-- holds under concurrent uploads (builds are immutable: no row lock).
SELECT pg_advisory_xact_lock(hashtextextended('context.build:' || sqlc.arg(build_id)::text, 0));

-- name: ListProjectBuildSummaries :many
-- Every build of a project, which retention bounds: what the current
-- views and retention are computed from.
SELECT id, application_id, source, branch, on_default_branch, created_at
FROM context_builds
WHERE project_id = sqlc.arg(project_id)
ORDER BY application_id, source, created_at DESC, id DESC;

-- name: ListBuildsByIDs :many
SELECT * FROM context_builds WHERE id = ANY(sqlc.arg(ids)::uuid[]) ORDER BY application_id, source;

-- name: DeleteBuilds :execrows
-- Usages, captures and regions go with their builds (ON DELETE CASCADE).
DELETE FROM context_builds WHERE id = ANY(sqlc.arg(ids)::uuid[]);

-- name: DeleteProjectBuilds :exec
DELETE FROM context_builds WHERE project_id = sqlc.arg(project_id);

-- name: DeleteApplicationBuilds :exec
DELETE FROM context_builds WHERE project_id = sqlc.arg(project_id) AND application_id = sqlc.arg(application_id);
