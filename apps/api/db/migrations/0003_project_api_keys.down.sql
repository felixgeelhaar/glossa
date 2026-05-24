-- Reverse 0003: restore projects.api_key_hash from the most-recent
-- non-revoked write-scope row per project, then drop the new table.
ALTER TABLE projects ADD COLUMN api_key_hash BYTEA;

UPDATE projects p
SET api_key_hash = sub.hash
FROM (
  SELECT DISTINCT ON (project_id) project_id, hash
  FROM project_api_keys
  WHERE revoked_at IS NULL AND scope = 'write'
  ORDER BY project_id, created_at DESC
) AS sub
WHERE p.id = sub.project_id;

ALTER TABLE projects ALTER COLUMN api_key_hash SET NOT NULL;

DROP TABLE IF EXISTS project_api_keys;
