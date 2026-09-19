-- 0017 — the daily purge job: a cluster-wide lease and the tenant
-- sweep behind Catalog's proposal sweep (RFC 0004 §2.3, §4.1).
--
-- glossa-server runs several replicas, and the purge is not something
-- to run twice at once: each periodic job takes a named lease first,
-- like a worker claims a job. The lease is a single row per job, held
-- for as long as the run may take, with the last completed run
-- recorded so a replica that starts a minute later doesn't purge
-- again.

-- ── leases ─────────────────────────────────────────────────────────
-- Tenantless: a lease belongs to the deployment, not to a tenant. It
-- is reached only as glossa_system (scope "scheduler.lease"), so
-- glossa_app gets no grant at all and row-level security has nothing
-- tenant-shaped to enforce.

CREATE TABLE system_leases (
    name        text        PRIMARY KEY CHECK (name ~ '^[a-z][a-z0-9_.-]{0,62}$'),
    holder      text        NOT NULL DEFAULT '' CHECK (octet_length(holder) <= 200),
    acquired_at timestamptz,
    expires_at  timestamptz,
    -- When the job last finished a run. Due-ness is measured from it,
    -- so an interval survives restarts and rolling upgrades.
    last_run_at timestamptz
);

ALTER TABLE system_leases ENABLE ROW LEVEL SECURITY;
ALTER TABLE system_leases FORCE ROW LEVEL SECURITY;
CREATE POLICY system_leases_system ON system_leases TO glossa_system USING (true) WITH CHECK (true);
GRANT SELECT, INSERT, UPDATE ON system_leases TO glossa_system;

-- ── the proposal sweep's tenant list ───────────────────────────────
-- Catalog's sweep obsoletes a closed branch's proposed messages 14 days
-- after it closed (RFC 0004 §4.1). It runs per tenant, in that tenant's
-- scope; the purge job finds which tenants have a closed branch at all
-- (system scope "catalog.proposals"), and learns nothing else about
-- them.

CREATE POLICY catalog_branches_system_select ON catalog_branches
    FOR SELECT TO glossa_system USING (true);
GRANT SELECT (tenant_id, closed_at) ON catalog_branches TO glossa_system;
