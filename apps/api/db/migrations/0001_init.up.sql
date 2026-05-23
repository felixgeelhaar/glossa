-- Initial Glossa schema. Multi-tenant from day one: every authed
-- request derives a tenant from its API key or JWT, and Row-Level
-- Security policies on every table enforce isolation at the DB layer
-- (a buggy handler that forgets the WHERE tenant_id = ... still
-- cannot read across tenants).
--
-- See docs/design.md §6 for the full schema rationale.

CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE tenants (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  slug        VARCHAR(50) UNIQUE NOT NULL,
  name        VARCHAR(200) NOT NULL,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE projects (
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id       UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  slug            VARCHAR(50) NOT NULL,
  name            VARCHAR(200) NOT NULL,
  default_locale  VARCHAR(8) NOT NULL DEFAULT 'de',
  api_key_hash    BYTEA NOT NULL,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (tenant_id, slug)
);

CREATE INDEX idx_projects_tenant ON projects (tenant_id);

CREATE TABLE locales (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id  UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  code        VARCHAR(8) NOT NULL,
  label       VARCHAR(50) NOT NULL,
  enabled     BOOLEAN NOT NULL DEFAULT TRUE,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (project_id, code)
);

CREATE INDEX idx_locales_project ON locales (project_id);

CREATE TABLE keys (
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id      UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  key             VARCHAR(255) NOT NULL,
  description     TEXT,
  first_seen_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (project_id, key)
);

CREATE INDEX idx_keys_project ON keys (project_id);

CREATE TABLE translations (
  id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  key_id       UUID NOT NULL REFERENCES keys(id) ON DELETE CASCADE,
  locale_id    UUID NOT NULL REFERENCES locales(id) ON DELETE CASCADE,
  value        TEXT NOT NULL,
  status       VARCHAR(20) NOT NULL DEFAULT 'pending',
  updated_by   UUID,
  updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (key_id, locale_id),
  CONSTRAINT translations_status_check
    CHECK (status IN ('pending', 'needs_review', 'approved'))
);

CREATE INDEX idx_translations_key_locale ON translations (key_id, locale_id);
CREATE INDEX idx_translations_open ON translations (status)
  WHERE status <> 'approved';

CREATE TABLE users (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id     UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  email         VARCHAR(255) NOT NULL,
  password_hash BYTEA NOT NULL,
  role          VARCHAR(20) NOT NULL DEFAULT 'translator',
  locales       TEXT[] NOT NULL DEFAULT '{}',
  created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (tenant_id, email),
  CONSTRAINT users_role_check
    CHECK (role IN ('admin', 'translator'))
);

CREATE INDEX idx_users_tenant ON users (tenant_id);

CREATE TABLE audit_log (
  id              BIGSERIAL PRIMARY KEY,
  tenant_id       UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  translation_id  UUID,
  before_value    TEXT,
  after_value     TEXT,
  changed_by      UUID,
  changed_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_audit_log_tenant_time
  ON audit_log (tenant_id, changed_at DESC);

-- ── Row-Level Security ────────────────────────────────────────────
-- Every queryable table carries a tenant scope (directly or via a
-- traversable FK). Policies key off app.current_tenant which the API
-- sets via `SET LOCAL` at the start of each request, after auth has
-- resolved the tenant from the API key / JWT.

ALTER TABLE tenants       ENABLE ROW LEVEL SECURITY;
ALTER TABLE projects      ENABLE ROW LEVEL SECURITY;
ALTER TABLE locales       ENABLE ROW LEVEL SECURITY;
ALTER TABLE keys          ENABLE ROW LEVEL SECURITY;
ALTER TABLE translations  ENABLE ROW LEVEL SECURITY;
ALTER TABLE users         ENABLE ROW LEVEL SECURITY;
ALTER TABLE audit_log     ENABLE ROW LEVEL SECURITY;

-- Tenants table: a row is visible to its own tenant only.
CREATE POLICY tenants_isolation ON tenants
  USING (id::text = current_setting('app.current_tenant', true));

-- Projects: direct tenant_id column.
CREATE POLICY projects_isolation ON projects
  USING (tenant_id::text = current_setting('app.current_tenant', true));

-- Locales / keys: traverse the project FK to the tenant_id.
CREATE POLICY locales_isolation ON locales
  USING (EXISTS (
    SELECT 1 FROM projects p
    WHERE p.id = locales.project_id
      AND p.tenant_id::text = current_setting('app.current_tenant', true)
  ));

CREATE POLICY keys_isolation ON keys
  USING (EXISTS (
    SELECT 1 FROM projects p
    WHERE p.id = keys.project_id
      AND p.tenant_id::text = current_setting('app.current_tenant', true)
  ));

-- Translations: traverse key → project → tenant_id.
CREATE POLICY translations_isolation ON translations
  USING (EXISTS (
    SELECT 1 FROM keys k
    JOIN projects p ON p.id = k.project_id
    WHERE k.id = translations.key_id
      AND p.tenant_id::text = current_setting('app.current_tenant', true)
  ));

-- Users + audit_log: direct tenant_id column.
CREATE POLICY users_isolation ON users
  USING (tenant_id::text = current_setting('app.current_tenant', true));

CREATE POLICY audit_log_isolation ON audit_log
  USING (tenant_id::text = current_setting('app.current_tenant', true));
