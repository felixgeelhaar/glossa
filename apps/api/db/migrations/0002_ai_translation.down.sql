ALTER TABLE audit_log DROP CONSTRAINT IF EXISTS audit_log_actor_kind_check;
ALTER TABLE audit_log DROP COLUMN IF EXISTS actor_label;
ALTER TABLE audit_log DROP COLUMN IF EXISTS actor_kind;

ALTER TABLE translations DROP CONSTRAINT translations_status_check;
ALTER TABLE translations ADD CONSTRAINT translations_status_check
  CHECK (status IN ('pending', 'needs_review', 'approved'));

DROP TABLE IF EXISTS ai_translation_providers;
