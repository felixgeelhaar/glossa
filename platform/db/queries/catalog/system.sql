-- System scope catalog.proposals (db.SystemTx as glossa_system):
-- migration 0017 opens catalog_branches' tenant_id and closed_at to it,
-- read-only, and nothing else. The daily purge job uses it to find
-- which tenants have a closed branch at all, then sweeps each in its
-- own tenant scope (RFC 0004 §4.1).

-- name: ListTenantsWithClosedBranches :many
SELECT DISTINCT tenant_id FROM catalog_branches WHERE closed_at IS NOT NULL ORDER BY tenant_id;
-- System scope (db.SystemTx as glossa_system): migration 0016 opens
-- these columns to it, read-only, and nothing else. The query finds
-- work across tenants; the sweep itself runs in each tenant's scope.

-- name: ListTenantsWithExpiredProposals :many
-- System scope catalog.proposal_sweep: the tenants holding proposed
-- messages whose every proposing branch closed or merged before the
-- cutoff — exactly what SweepProposals obsoletes.
SELECT DISTINCT m.tenant_id FROM catalog_messages m
WHERE m.state = 'proposed'
  AND EXISTS (SELECT FROM catalog_proposals p WHERE p.message_id = m.id AND p.kind = 'new_key')
  AND NOT EXISTS (SELECT FROM catalog_proposals p JOIN catalog_branches b ON b.id = p.branch_id
                  WHERE p.message_id = m.id AND p.kind = 'new_key'
                    AND (b.closed_at IS NULL OR b.closed_at > sqlc.arg(cutoff)))
ORDER BY m.tenant_id
LIMIT sqlc.arg(max_rows)::int;
