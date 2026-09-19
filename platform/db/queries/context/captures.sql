-- Tenant scope (db.TenantTx): RLS limits every statement to the current
-- tenant.

-- name: InsertCapture :execrows
-- One capture per (route, viewport, locale) of a build: a repeat
-- inserts nothing and the caller compares it with the first.
INSERT INTO context_captures (id, tenant_id, build_id, project_id, route, viewport_width, viewport_height, locale,
                              image_digest, image_width, image_height, created_by, created_at)
VALUES (sqlc.arg(id), app_current_tenant(), sqlc.arg(build_id), sqlc.arg(project_id), sqlc.arg(route),
        sqlc.arg(viewport_width), sqlc.arg(viewport_height), sqlc.arg(locale), sqlc.arg(image_digest),
        sqlc.arg(image_width), sqlc.arg(image_height), sqlc.arg(created_by), sqlc.arg(created_at))
ON CONFLICT (build_id, route, viewport_width, viewport_height, locale) DO NOTHING;

-- name: GetCaptureByShot :one
SELECT * FROM context_captures
WHERE build_id = sqlc.arg(build_id) AND route = sqlc.arg(route) AND viewport_width = sqlc.arg(viewport_width)
  AND viewport_height = sqlc.arg(viewport_height) AND locale = sqlc.arg(locale);

-- name: CountBuildCaptures :one
SELECT count(*) FROM context_captures WHERE build_id = sqlc.arg(build_id);

-- name: InsertRegions :exec
INSERT INTO context_regions (capture_id, position, tenant_id, message_key, message_id, kind, x, y, width, height, visible)
SELECT sqlc.arg(capture_id), r.position, app_current_tenant(), r.message_key,
       NULLIF(r.message_id, '00000000-0000-0000-0000-000000000000'::uuid), r.kind, r.x, r.y, r.width, r.height,
       r.visible
FROM (SELECT unnest(sqlc.arg(positions)::int[]) AS position, unnest(sqlc.arg(keys)::text[]) AS message_key,
             unnest(sqlc.arg(message_ids)::uuid[]) AS message_id, unnest(sqlc.arg(kinds)::text[]) AS kind,
             unnest(sqlc.arg(xs)::int[]) AS x, unnest(sqlc.arg(ys)::int[]) AS y,
             unnest(sqlc.arg(widths)::int[]) AS width, unnest(sqlc.arg(heights)::int[]) AS height,
             unnest(sqlc.arg(visibles)::boolean[]) AS visible) AS r;

-- name: ListRegions :many
SELECT * FROM context_regions WHERE capture_id = sqlc.arg(capture_id) ORDER BY position;

-- name: ListBuildImages :many
-- The images the given builds' captures reference.
SELECT DISTINCT image_digest FROM context_captures WHERE build_id = ANY(sqlc.arg(build_ids)::uuid[]);

-- name: ListReferencedImages :many
-- Which of digests a capture of the project still references.
SELECT DISTINCT image_digest FROM context_captures
WHERE project_id = sqlc.arg(project_id) AND image_digest = ANY(sqlc.arg(digests)::text[]);
