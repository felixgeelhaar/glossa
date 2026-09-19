-- System scope catalog.proposals (db.SystemTx as glossa_system):
-- migration 0017 opens catalog_branches' tenant_id and closed_at to it,
-- read-only, and nothing else. The daily purge job uses it to find
-- which tenants have a closed branch at all, then sweeps each in its
-- own tenant scope (RFC 0004 §4.1).

-- name: ListTenantsWithClosedBranches :many
SELECT DISTINCT tenant_id FROM catalog_branches WHERE closed_at IS NOT NULL ORDER BY tenant_id;
