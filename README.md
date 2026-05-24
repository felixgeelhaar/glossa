# Glossa

> Self-hosted, multi-tenant translation management. REST API + SSE live updates + Lit admin UI + optional AI translator agents.

![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)
![Go](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white)
![Postgres](https://img.shields.io/badge/Postgres-16-336791?logo=postgresql&logoColor=white)
![Lit](https://img.shields.io/badge/Lit-3-324FFF?logo=lit&logoColor=white)

Glossa is the translation-management backbone for [Brotwerk](https://brotwerk.felixgeelhaar.de), [IRI](https://github.com/felixgeelhaar/iri), and Kraftsport. One deployment, many tenants, end-to-end German-first but locale-agnostic.

---

## Features

- **Multi-tenant from day one.** Row-Level Security on every queryable table — a buggy handler that forgets `WHERE tenant_id = …` still cannot read across tenants. Tenancy is enforced via `SET LOCAL app.current_tenant` in a tx per request.
- **REST + SSE.** Consumers fetch bundles over HTTP and subscribe to live updates over Server-Sent Events. Edit a key in the admin → connected clients render the new string within ~1s.
- **Editorial lifecycle.** Translations move through `pending → ai_translated → needs_review → approved`. Status pills in the UI; status filter in the editor.
- **AI translator agents (optional).** Configure OpenAI / Anthropic / Gemini / OpenAI-compatible endpoints per tenant. When a source-locale write lands, every other enabled locale gets an `ai_translated` row for reviewer approval. Existing approved / needs_review rows are never overwritten. API keys are AES-GCM encrypted at rest with `GLOSSA_SECRETS_KEY`.
- **Email-first auth.** Login takes (email, password) — the tenant is inferred. Translators are scoped to specific locales; admins can do everything.
- **Audit log.** Every translation mutation is recorded with before/after value, actor (`user` / `ai` / `system`), and timestamp.
- **Design system.** `@glossa/ui` ships Lit primitives (`gl-button`, `gl-input`, `gl-select`, `gl-table`, `gl-badge`, …) with light/dark/system theming. The admin UI is built from those primitives.
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
│  apps/admin  (Lit + Vite + @glossa/ui)                 │
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

## Quick start (Docker)

```bash
git clone https://github.com/felixgeelhaar/glossa
cd glossa
docker compose up --build
```

Boots Postgres 16, runs migrations, starts the API, and serves the admin at <http://localhost:5173>. The first run bootstraps a `demo` tenant with admin `felix@example.com` / `hunter2hunter2`.

To enable the AI translator, the dev compose file already provides a non-secret `GLOSSA_SECRETS_KEY`. Add a provider in the **AI translation** tab and any source-locale (project default) write will fan out.

---

## Project layout

```
glossa/
├── apps/
│   ├── api/                    # Go service: hex arch (domain → app → interfaces → infra)
│   │   ├── cmd/api/            # binary entry
│   │   ├── db/migrations/      # numbered SQL migrations (run via `migrate`)
│   │   ├── db/queries/         # sqlc input
│   │   └── internal/
│   │       ├── domain/         # aggregates + repository ports
│   │       ├── app/            # use cases (per-feature subpackage)
│   │       ├── interfaces/     # gin handlers
│   │       └── infra/          # sqlc adapter, AES-GCM secrets, AI clients
│   └── admin/                  # Lit SPA, served by nginx in compose
├── packages/
│   └── ui/                     # @glossa/ui — design system primitives
├── deploy/k3s/                 # k3s manifests + Helm-free kustomize bases
├── docs/                       # design doc + ADRs
└── docker-compose.yml          # one-command dev stack
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

# Admin SPA
cd apps/admin
pnpm install
pnpm dev   # http://localhost:5173 against running compose api

# Design system
cd packages/ui
pnpm build
```

---

## Roadmap

| Status | Item |
|---|---|
| ✅ shipped | Multi-tenant API + Postgres schema + RLS |
| ✅ shipped | Admin SPA + design system + dark mode |
| ✅ shipped | Email-first login + tenant inference |
| ✅ shipped | AI translator agents (OpenAI / Anthropic / Gemini) |
| ✅ shipped | SSE live updates |
| ✅ shipped | Audit log + actor attribution |
| 🚧 next   | `packages/sdk` + `packages/elements` + `packages/cli` for consumer apps |
| 🚧 next   | AI backfill button (translate every missing key in one pass) |
| 🚧 next   | Translation memory across projects in a tenant |
| 🔭 later  | DeepL passthrough as an alternative provider kind |
| 🔭 later  | Plurals editor in admin (visual ICU builder) |

---

## License

[MIT](LICENSE)
