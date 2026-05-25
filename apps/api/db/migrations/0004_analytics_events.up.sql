-- Analytics events. Captures the cold-start funnel so we can answer
-- "how long does it take a fresh project to land its first
-- approved translation" — the metric every roadmap call now hinges
-- on. Append-only ledger; read-time aggregation handles 'first
-- occurrence' detection without a dual-write race.

CREATE TABLE analytics_events (
  id          BIGSERIAL PRIMARY KEY,
  tenant_id   UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  project_id  UUID REFERENCES projects(id) ON DELETE CASCADE,
  kind        VARCHAR(64) NOT NULL,
  metadata    JSONB NOT NULL DEFAULT '{}'::jsonb,
  occurred_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT analytics_events_kind_check
    CHECK (kind IN (
      'project_created',
      'first_key_synced',
      'first_translation_edited',
      'first_consumer_request',
      'first_ai_translation',
      'translation_edited',
      'consumer_request',
      'ai_translation'
    ))
);

-- Funnel-read pattern: per (tenant_id, project_id, kind) we always
-- want MIN(occurred_at). A covering index keeps the cohort query
-- O(log n) per project.
CREATE INDEX idx_analytics_events_funnel
  ON analytics_events (tenant_id, project_id, kind, occurred_at);

-- Time-series scans for usage volume.
CREATE INDEX idx_analytics_events_recent
  ON analytics_events (tenant_id, occurred_at DESC);

ALTER TABLE analytics_events ENABLE ROW LEVEL SECURITY;
CREATE POLICY analytics_events_isolation ON analytics_events
  USING (tenant_id::text = current_setting('app.current_tenant', true));
