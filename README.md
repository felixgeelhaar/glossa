# Glossa

> Cross-project translation management — REST API + Lit web components + own ICU subset.

**Status**: planning. Build starts after IRI v0.2.0 stabilizes.

## What

Translation management service that any project can consume:

- **Backend** (Go + Postgres) — REST + SSE; multi-tenant from day one
- **Admin UI** (Lit + Vite) — translator interface
- **Web components** (`@felixgeelhaar/glossa-elements`) — `<glossa-provider>`, `<glossa-text>`, `<glossa-rich>` drop into any framework (Vue, React, Svelte, Astro, plain HTML)
- **ICU subset** (`@felixgeelhaar/glossa-format`, ~200 LOC) — vars + plurals via `Intl.PluralRules` + select + nesting. No `@formatjs/intl-messageformat` dependency.
- **CLI** (`@felixgeelhaar/glossa-cli`) — `glossa init / scan / pull / push`

## Why

IRI (sister project) just shipped 170+ files with hardcoded German strings. Adding EN / FR / IT requires re-editing all of them. Brotwerk + future projects face the same problem.

Tolgee / Lokalise / Crowdin are options but:
- Want cross-project translation memory in one place
- Want full control over component API (Lit web components ship to any framework)
- Want EU-hosted, MIT-licensed, self-controllable

## Decisions locked (2026-05-23)

| | |
|---|---|
| Repo | `github.com/felixgeelhaar/glossa` (private at start, public when stable) |
| License | MIT |
| Tenancy | Multi-tenant from day one |
| Subdomain | TBD — `glossa.app` / `.io` / `.dev` all camped, alternative naming planned |
| Build sequencing | IRI stabilization clears first, then Glossa kickoff |

## Architecture

```
┌─ Glossa Service ─────────────────────────────────┐
│  Backend (Go + Postgres)                          │
│   ├── REST API: projects, locales, keys, trans   │
│   ├── SSE channel: live translation updates      │
│   └── Optional: DeepL passthrough for MT drafts  │
│                                                   │
│  Admin UI (Lit + Vite, served at /admin)         │
│   ├── Translator interface (key list, edit form) │
│   ├── Project management                         │
│   ├── Diff view (untranslated vs needs-review)   │
│   └── Bulk import/export JSON                    │
└───────────────────────────────────────────────────┘
                │
                │ HTTPS + SSE
                ▼
┌─ Glossa Lit Components (npm package) ────────────┐
│  <glossa-provider>          // root: project +    │
│                              // locale + api-url  │
│  <glossa-text key="...">    // simple translation │
│  <glossa-rich key="...">    // ICU MessageFormat  │
│  <glossa-plural>            // plural variants    │
│  <glossa-select>            // gender/select vars │
│                                                   │
│  Fallback rendering: slot content shown while     │
│  loading or if key missing                        │
└───────────────────────────────────────────────────┘
                │
                │ drops into any framework
                ▼
   ┌─────────┬─────────┬─────────┬─────────┐
   │  IRI    │ Brotwerk│ Future1 │ Future2 │
   │ Astro+V │  Astro  │  Next   │  Svelte │
   └─────────┴─────────┴─────────┴─────────┘
```

## Monorepo layout (pnpm workspaces)

```
glossa/
├── apps/
│   ├── api/                    # Go service (REST + SSE + admin auth)
│   └── admin/                  # Lit admin UI
├── packages/
│   ├── elements/               # @felixgeelhaar/glossa-elements
│   ├── format/                 # @felixgeelhaar/glossa-format (ICU subset)
│   ├── sdk/                    # @felixgeelhaar/glossa-sdk (plain JS)
│   └── cli/                    # @felixgeelhaar/glossa-cli
├── deploy/k3s/                 # k3s manifests (mirror IRI patterns)
├── .github/workflows/release.yml
├── docs/
│   ├── design.md               # full design doc (see /docs)
│   ├── adr/0001-monorepo.md
│   ├── adr/0002-icu-subset.md
│   ├── adr/0003-lit-components.md
│   └── api.md
├── pnpm-workspace.yaml
└── README.md
```

## 10-day MVP plan

| Day | Work |
|---|---|
| 1 | Repo scaffold + pnpm workspace + 3 ADRs (monorepo / ICU subset / Lit choice) |
| 2 | `packages/format` — ICU subset parser + tests (vars / plurals via `Intl.PluralRules` / select / nesting) |
| 3 | `packages/sdk` — fetch client + in-memory cache + SSE subscriber |
| 4 | `packages/elements` — `<glossa-provider>` + `<glossa-text>` + `<glossa-rich>` |
| 5 | Go API — Postgres schema (multi-tenant), REST endpoints, JWT |
| 6 | Go API — SSE channel + Redis fanout |
| 7 | `apps/admin` — translator UI (login, key list, edit modal, locale switcher) |
| 8 | `packages/cli` — `glossa init / scan / pull / push` |
| 9 | k3s deploy + GH Actions release (mirror IRI patterns) |
| 10 | Wire IRI as first consumer — replace ~20 strings, verify end-to-end |

## Sequencing

- **Blocked by**: IRI v0.2.0 stabilization (currently in production, awaiting BVDG-coach pilot feedback)
- **Blocks**: i18n framework introduction to IRI (Roady #52) — that work absorbs into Glossa adoption
- **Blocks**: bespoke API error sweep in IRI (Roady #53) — done as part of Glossa rollout

## Open questions

- Brand name / domain — `glossa` is the working name; all primary `.app/.io/.dev` TLDs camped. Alternative names on hunt list.
- Translator workflow — initial: manual git + admin UI edits. Post-MVP: Tolgee-style sync API for external translator tools.
- Machine translation passthrough — DeepL is the obvious EU pick; defer to post-MVP.
- Plurals editor in admin UI — visual ICU MessageFormat builder vs raw textarea. Raw for MVP.

## See also

- `docs/design.md` — full system design (long-form)
- Roady features #52, #53 (in IRI repo backlog) — track downstream work this enables
