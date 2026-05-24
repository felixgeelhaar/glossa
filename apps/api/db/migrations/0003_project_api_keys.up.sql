-- Per-project API keys with scopes.
--
-- Before: projects.api_key_hash held a single SHA-256 hash; whoever
-- had the raw key had full read+write on the project.
--
-- After: a separate table holds one row per issued key, each with
-- a scope ('read' or 'write') + a human-friendly label so admins
-- can name and rotate keys without losing context.
--
-- 'read' scope: GET endpoints only (bundle, SSE).
-- 'write' scope: everything 'read' plus PATCH (translations), POST
--                (keys scan, bulk import).
--
-- The existing single key on each project is migrated forward as a
-- 'write'-scope row labeled 'legacy' so already-shipped consumers
-- keep working. The projects.api_key_hash column is dropped — the
-- new table is now authoritative.

CREATE TABLE project_api_keys (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id    UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  hash          BYTEA NOT NULL,
  scope         VARCHAR(10) NOT NULL,
  label         VARCHAR(100) NOT NULL,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  last_used_at  TIMESTAMPTZ,
  revoked_at    TIMESTAMPTZ,
  CONSTRAINT project_api_keys_scope_check
    CHECK (scope IN ('read', 'write'))
);

-- Hash lookup runs on every API-key authenticated request — the
-- index is critical. Partial: revoked rows are out of scope so
-- they're excluded from the index to keep it tight.
CREATE UNIQUE INDEX idx_project_api_keys_hash
  ON project_api_keys (hash)
  WHERE revoked_at IS NULL;

CREATE INDEX idx_project_api_keys_project
  ON project_api_keys (project_id);

ALTER TABLE project_api_keys ENABLE ROW LEVEL SECURITY;
CREATE POLICY project_api_keys_isolation ON project_api_keys
  USING (EXISTS (
    SELECT 1 FROM projects p
    WHERE p.id = project_api_keys.project_id
      AND p.tenant_id::text = current_setting('app.current_tenant', true)
  ));

-- Carry every existing project's single key forward as a 'write'
-- row so SDK consumers + the CLI keep working without code change.
-- Label 'legacy' makes the migration self-documenting in the UI.
INSERT INTO project_api_keys (project_id, hash, scope, label)
SELECT id, api_key_hash, 'write', 'legacy'
FROM projects;

-- Drop the column now that the table is authoritative. Rollback path
-- (.down) reverses this by copying the most recent write-scope row
-- back into projects.api_key_hash.
ALTER TABLE projects DROP COLUMN api_key_hash;
