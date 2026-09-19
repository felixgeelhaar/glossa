REVOKE SELECT (tenant_id, closed_at) ON catalog_branches FROM glossa_system;
DROP POLICY IF EXISTS catalog_branches_system_select ON catalog_branches;

DROP TABLE IF EXISTS system_leases;
