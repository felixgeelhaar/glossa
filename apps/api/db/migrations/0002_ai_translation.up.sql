-- AI translator support.
--
-- ai_translation_providers stores per-tenant LLM provider configs. The
-- raw API key is never persisted; only the AES-GCM ciphertext + nonce
-- live in the row. Decryption keys come from the GLOSSA_SECRETS_KEY env
-- (32-byte hex), derived at process start.
--
-- The `ai_translated` translation status sits between `pending` and
-- `needs_review`: the agent produced something usable, but a human has
-- not signed off yet.
--
-- audit_log gains an actor_kind + actor_label so a row produced by an
-- agent (changed_by IS NULL, actor_kind = 'ai', actor_label = 'openai')
-- is distinguishable from a deleted-user row.

CREATE TABLE ai_translation_providers (
  id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id         UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  kind              VARCHAR(20) NOT NULL,
  label             VARCHAR(100) NOT NULL,
  base_url          TEXT NOT NULL DEFAULT '',
  model             VARCHAR(100) NOT NULL,
  api_key_ct        BYTEA NOT NULL,
  api_key_nonce     BYTEA NOT NULL,
  enabled           BOOLEAN NOT NULL DEFAULT TRUE,
  created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT ai_providers_kind_check
    CHECK (kind IN ('openai', 'anthropic', 'gemini', 'custom'))
);

CREATE INDEX idx_ai_providers_tenant ON ai_translation_providers (tenant_id);
CREATE INDEX idx_ai_providers_enabled ON ai_translation_providers (tenant_id, enabled)
  WHERE enabled = TRUE;

ALTER TABLE ai_translation_providers ENABLE ROW LEVEL SECURITY;
CREATE POLICY ai_providers_isolation ON ai_translation_providers
  USING (tenant_id::text = current_setting('app.current_tenant', true));

-- Extend status enum.
ALTER TABLE translations DROP CONSTRAINT translations_status_check;
ALTER TABLE translations ADD CONSTRAINT translations_status_check
  CHECK (status IN ('pending', 'ai_translated', 'needs_review', 'approved'));

-- Audit attribution for non-human actors.
ALTER TABLE audit_log
  ADD COLUMN actor_kind  VARCHAR(20) NOT NULL DEFAULT 'user',
  ADD COLUMN actor_label VARCHAR(100) NOT NULL DEFAULT '';

ALTER TABLE audit_log
  ADD CONSTRAINT audit_log_actor_kind_check
    CHECK (actor_kind IN ('user', 'ai', 'system'));
