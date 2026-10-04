# Glossa

> **For product teams who want every new language to be configuration, not an engineering project, Glossa is open-source localization infrastructure: one typed message model for web and backend, AI translation grounded in your terminology and translation memory, and immutable releases delivered to every runtime.** Unlike file-centric translation tools, Glossa treats translations as versioned product data you own: self-hosted, with your own LLM keys.

**Build once. Speak everywhere.** Where Glossa is going and why: [`docs/product-intent.md`](./docs/product-intent.md). How it gets there: [RFC 0002 — Platform architecture](./docs/rfcs/0002-platform-architecture.md), a rewrite. The feature list and architecture below describe v0.3, which is retired (see *Roadmap*); the platform's own docs are `platform/README.md` and `platform/cmd/glossa/README.md`.

![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)
![Go](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white)
![Postgres](https://img.shields.io/badge/Postgres-16-336791?logo=postgresql&logoColor=white)
![Lit](https://img.shields.io/badge/Lit-3-324FFF?logo=lit&logoColor=white)

Glossa is the localization backbone for [Brotwerk](https://brotwerk.felixgeelhaar.de), [IRI](https://github.com/felixgeelhaar/iri), and [Kraftsport](https://kraftsport-coach.de). One deployment, many tenants. Full positioning rationale in [`docs/positioning.md`](./docs/positioning.md).

---

## Features

- **Multi-tenant from day one.** Row-Level Security on every queryable table — a buggy handler that forgets `WHERE tenant_id = …` still cannot read across tenants. Tenancy is enforced via `SET LOCAL app.current_tenant` in a tx per request.
- **REST + SSE.** Consumers fetch bundles over HTTP and subscribe to live updates over Server-Sent Events. Edit a key in the admin → connected clients render the new string within ~1s.
- **Editorial lifecycle.** Translations move through `pending → ai_translated → needs_review → approved`. Status pills in the UI; status filter in the editor.
- **AI translator agents (optional).** Configure OpenAI / Anthropic / Gemini / OpenAI-compatible endpoints per tenant. When a source-locale write lands, every other enabled locale gets an `ai_translated` row for reviewer approval. Existing approved / needs_review rows are never overwritten. API keys are AES-GCM encrypted at rest with `GLOSSA_SECRETS_KEY`.
- **Email-first auth.** Login takes (email, password) — the tenant is inferred. Translators are scoped to specific locales; admins can do everything.
- **Audit log.** Every translation mutation is recorded with before/after value, actor (`user` / `ai` / `system`), and timestamp.
- **Design system.** `@felixgeelhaar/glossa-ui` ships Lit primitives (`gl-button`, `gl-input`, `gl-select`, `gl-table`, `gl-badge`, …) with light/dark/system theming. The admin UI is built from those primitives.
- **Bulk import / export.** Atomic upsert of full `{key: value}` bundles; per-row failures reported alongside successes.
- **Diff view.** Per-locale untranslated + needs-review counts at a glance.

---

## Architecture

```
┌─ Glossa Service ──────────────────────────────────────┐
│                                                        │
│  apps/api  (Go + pgx/v5 + sqlc + gin)                  │
│   ├── REST: projects / locales / keys / translations   │
│   ├── SSE: live translation updates per (project,tnt)  │
│   ├── Auth: JWT (admin SPA) + API key (consumer SDK)   │
│   └── AI fan-out: source-locale write → N targets      │
│                                                        │
│  apps/admin  (Lit + Vite + @felixgeelhaar/glossa-ui)                 │
│   ├── Editor / Bulk / Diff / Locales / Users           │
│   ├── AI translation (provider config + test)          │
│   └── Audit log                                        │
│                                                        │
│  packages/ui  (Lit primitives + tokens)                │
│                                                        │
└────────────────────────────────────────────────────────┘
                       │
                       │ HTTPS + SSE
                       ▼
        ┌─────────────┬──────────────┬─────────────┐
        │  Brotwerk   │     IRI      │  Kraftsport │
        │   Astro     │  Astro+Vue   │     TBD     │
        └─────────────┴──────────────┴─────────────┘
```

---

## Running v0.3

v0.3's admin SPA, container images, Helm chart, k3s manifests and compose stack were deleted when it was retired (RFC 0006 §7). They are in the history before the commit that merged `m5/retire-v0`. The platform deploys from [`deploy/charts/glossa-platform`](./deploy/charts/glossa-platform).

---

## Project layout

```
glossa/
├── apps/api/                   # v0.3's Go service, kept only as the importer's and the M5 exit test's fixture
├── packages/format/            # @felixgeelhaar/glossa-format: v0.3's ICU formatter, what `glossa import --from v0 --verify` compares against
├── platform/  messageformat/  runtimes/  studio/  site/   # the rewrite (RFC 0002)
├── deploy/charts/glossa-platform/   # the platform's chart
└── docs/                       # product intent, positioning, design doc, RFCs, runbooks
```

---

## API surface (v1)

### Consumer (API-key Bearer)
| Method | Path | Purpose |
|---|---|---|
| `GET`  | `/api/v1/projects/:slug/locales/:locale/messages` | Bundle export |
| `GET`  | `/api/v1/projects/:slug/sse` | Live updates |
| `PATCH`| `/api/v1/projects/:slug/locales/:locale/keys/:key` | Update a translation |
| `POST` | `/api/v1/projects/:slug/keys:scan` | Idempotent key seeding |

### Admin (JWT)
| Method | Path | Role |
|---|---|---|
| `GET / POST`   | `/api/v1/admin/projects` | admin |
| `GET / PATCH`  | `/api/v1/admin/projects/:slug/locales/...` | translator + admin |
| `POST`         | `/api/v1/admin/projects/:slug/locales/:locale/bulk` | admin |
| `GET / POST / PATCH / DELETE` | `/api/v1/admin/users` | admin |
| `GET / POST / PATCH / DELETE` | `/api/v1/admin/ai-providers` | admin |
| `POST`         | `/api/v1/admin/ai-providers/:id/test` | admin |
| `GET`          | `/api/v1/admin/audit` | admin |

---

## Security

- API keys (tenant-scoped) are stored as SHA-256 hashes; comparison is constant-time at the driver level (Postgres byte equality on fixed-length BYTEA).
- AI provider credentials are AES-256-GCM encrypted per-row with a fresh 12-byte nonce. The master key (`GLOSSA_SECRETS_KEY`, 64-char hex) lives in env only; plaintext lives in process memory just long enough to call the upstream LLM.
- Login is rate-limited (5 req/min per IP, burst 10) to defend bcrypt against brute force.
- All authed requests run inside a transaction with `SET LOCAL app.current_tenant` — RLS policies on every table enforce isolation.

---

## Development

```bash
# Backend
cd apps/api
go test ./...
sqlc generate
```

---

## Roadmap

Glossa is being **rewritten** as the platform the [product intent](./docs/product-intent.md) describes, and the Klarlabs products are the first users. Architecture, milestones and adoption waves: [RFC 0002](./docs/rfcs/0002-platform-architecture.md). v0.3 is retired by [the runbook](./docs/runbooks/retire-v0.md); this change deleted its admin, SDKs, charts, manifests and compose stack once every former project had moved. `apps/api` and `packages/format` stay as test fixtures for the importer and the M5 exit test (RFC 0006 §7.3, §12.6) and receive no other work. Moving a project off v0.3 and retiring it: [`docs/runbooks/retire-v0.md`](./docs/runbooks/retire-v0.md).

| Milestone | Delivers | Done when |
|---|---|---|
| M0 Foundations | Server kernel, forced RLS, `auth-go`, MessageFormat 2 kernel (Go + TS) with conformance suite | Conformance + RLS suites green |
| M1 Core loop | Messages → translations → immutable releases → edge → JS / web components / Vue / Astro / Go runtimes, CLI, Studio v0 | Brotwerk runs on it, web and email |
| M2 Knowledge + AI | TM, termbase, style guides, translation agent with provenance and confidence, translator workspace | Armada's missing locales filled by review by exception |
| M3 Context | Usages, in-product editing, preview environments, screenshots, GitHub checks, React | Translators see where every message appears |
| M4 Quality | Layered QA, CI policies, visual QA, Flutter | `glossa check` gates CI in every product |
| M5 Operations | Workflows, assignments, vendors, audit export | All products migrated, v0.3 retired |

Each milestone has its design RFC: [M2 — 0003](./docs/rfcs/0003-knowledge-and-intelligence.md), [M3 — 0004](./docs/rfcs/0004-context.md), [M4 — 0005](./docs/rfcs/0005-quality.md), [M5 — 0006](./docs/rfcs/0006-operations.md).

---

## License

[MIT](LICENSE)
