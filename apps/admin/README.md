# `apps/admin` — Glossa admin UI (Lit + Vite)

Stub. Implementation lands after `apps/api` reaches MVP.

## What

Translator interface — list keys per project/locale, edit translations inline, bulk import/export JSON, diff "untranslated vs needs-review", manage projects + locales + API keys.

## Stack

- Lit 3 + Vite + TypeScript
- Uses `@glossa/sdk` (this monorepo) for API calls
- Uses `@glossa/elements` (this monorepo) to dogfood the same components consumer apps ship

## Status

- [ ] `pnpm init` + Lit/Vite scaffolding
- [ ] Auth (login, API-key rotation)
- [ ] Project + locale management views
- [ ] Translator key-edit form
- [ ] Diff / bulk-edit views
