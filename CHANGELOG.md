# Changelog

All notable changes to Glossa go here.

Format: [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) · SemVer.

## [Unreleased]

### Added

- **AI translator agents.** New `ai_translation_providers` table (AES-GCM
  encrypted credentials), new `ai_translated` status between `pending` and
  `needs_review`, fan-out worker on source-locale writes. OpenAI /
  Anthropic / Gemini / OpenAI-compatible custom endpoints supported.
  Admin UI tab for provider management + live test calls.
- **`@glossa/ui` design system.** Light / dark / system theme tokens,
  primitives (`gl-button`, `gl-input`, `gl-select`, `gl-textarea`,
  `gl-card`, `gl-badge`, `gl-table`, `gl-tabs`, `gl-toast`, `gl-toolbar`,
  `gl-theme-toggle`). Every admin tab migrated.
- **Email-first login.** `/auth/discover` returns the tenant list for an
  email; multi-tenant users pick which tenant to sign into. Empty result
  is returned for unknown emails to deny enumeration.
- **Docker Compose dev stack.** `docker compose up --build` brings up
  Postgres + migrate + API + admin. Admin nginx proxies `/api` over the
  compose network.
- Audit `before_value` is now correctly populated for translation edits.
- `actor_kind` + `actor_label` columns on `audit_log` for AI vs user
  attribution.
- `/auth/login` rate-limited (5 req/min per IP, burst 10) to defend
  bcrypt against brute force.

### Changed

- Audit log surfaces actor type alongside `changed_by` UUID — AI rows
  show their provider name; user rows show the email.

### Security

- AI provider API keys are AES-256-GCM encrypted at rest with per-row
  nonces. Master key (`GLOSSA_SECRETS_KEY`) is env-only.
- AI translation feature degrades gracefully: empty `GLOSSA_SECRETS_KEY`
  disables provider endpoints with a 503 + explanatory message.

## 0.0.0

Initial repo scaffold: monorepo layout (`apps/api`, `apps/admin`,
`packages/ui`), root `package.json` + `pnpm-workspace.yaml`, top-level
`Makefile`, MIT `LICENSE`, `.editorconfig`, `.gitignore`. Initial design
captured in `docs/design.md`.
