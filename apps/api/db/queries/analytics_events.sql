-- name: RecordAnalyticsEvent :exec
INSERT INTO analytics_events (tenant_id, project_id, kind, metadata)
VALUES ($1, $2, $3, $4);

-- name: ProjectFunnel :many
-- Returns first-occurrence timestamps per kind for a single project.
-- Drives the cohort funnel display in the admin metrics view.
SELECT kind, MIN(occurred_at) AS first_at, COUNT(*) AS total
FROM analytics_events
WHERE tenant_id = $1 AND project_id = $2
GROUP BY kind
ORDER BY kind;

-- name: TenantProjectsFirstEvents :many
-- One row per (project, kind) carrying the first-occurrence timestamp
-- across every project in the tenant. Used by the cohort dashboard
-- to compute time-to-first-X distributions.
SELECT project_id, kind, MIN(occurred_at) AS first_at
FROM analytics_events
WHERE tenant_id = $1 AND project_id IS NOT NULL
GROUP BY project_id, kind
ORDER BY project_id, kind;
