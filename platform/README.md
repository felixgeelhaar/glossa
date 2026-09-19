# platform

Go module for Glossa's control plane (RFC 0002). It holds the
`glossa-server` kernel (configuration, observability, the HTTP edge,
Postgres with forced row-level security, tenancy, the transactional
outbox), the `/v1` API contract, and the bounded contexts under
`internal/<context>/`: Identity, Catalog and Localization so far.

```text
api/openapi.yaml            the /v1 contract (OpenAPI 3.1), source of truth
cmd/glossa-server/          composition root only
db/migrations/              golang-migrate SQL, embedded into the binary
db/queries/<context>/       sqlc input, one directory per context
sqlc.yaml                   one sqlc entry per context
internal/apiv1/             Go server generated from api/openapi.yaml (oapi-codegen)
internal/kernel/
  config/                   env config, validated at startup
  observability/            bolt logger, OTel tracer, Prometheus, request middleware
  httpserver/               net/http server, /livez /readyz /metrics
  db/                       pgx pool, migrator, unit of work, dbtest harness
  tenancy/                  tenant ID/kind, context, Resolver middleware; tenantpg adapter
  outbox/                   Publish, Registry, Dispatcher, Postgres store
  problem/                  RFC 9457 error bodies with stable codes
  pagination/               page_size/page_token cursor pagination
  bcp47/                    canonical locale tags and text direction (shared kernel)
  mfcontent/                authored text + canonical MF2 model + derived metadata (shared kernel)
  etag/, idempotency/       ETag/If-Match and Idempotency-Key helpers
internal/apiv1/apiconv/     message content, QA findings and If-Match on the wire
internal/catalog/           projects, applications, messages, source revisions
internal/localization/      locales, fallback graphs, translations, revisions
internal/identity/
  domain/                   Person, Member, roles, locale scopes, Grant, APIToken, events
  app/                      sign-in flows (auth-go) and tenant/member/token use cases
  authz/                    authz.Require: the permission check every context uses
  adapters/postgres/        app + auth-go ports on the unit of work (sqlc)
  adapters/httpapi/         Identity's /v1 handlers, the Guard, the tenancy.Resolver
  adapters/mail/            log (dev) and SMTP mailers
  adapters/passkey/         auth-go's WebAuthn adapter, configured
```

## Running it

You need Postgres 16. Migrations run as the schema **owner**, a
non-superuser with `CREATEROLE`. The server connects as **`glossa_app`**,
which the first migration creates without a login.

```sh
docker run -d --name glossa-pg -p 5432:5432 -e POSTGRES_PASSWORD=postgres postgres:16-alpine
psql postgres://postgres:postgres@localhost:5432/postgres <<'SQL'
CREATE ROLE glossa_owner LOGIN CREATEROLE PASSWORD 'owner';
CREATE DATABASE glossa OWNER glossa_owner;
SQL

export MIGRATION_DATABASE_URL=postgres://glossa_owner:owner@localhost:5432/glossa?sslmode=disable
export DATABASE_URL=postgres://glossa_app:app@localhost:5432/glossa?sslmode=disable
export GLOSSA_AUTH_SECRET=$(openssl rand -base64 32)    # keep it: it seals TOTP secrets
go run ./cmd/glossa-server -migrate=only                 # creates roles, tables, policies
psql "$MIGRATION_DATABASE_URL" -c "ALTER ROLE glossa_app LOGIN PASSWORD 'app'"
go run ./cmd/glossa-server
```

With the default `GLOSSA_MAIL_DRIVER=log`, sign-in links are written to
the log instead of being mailed:

```sh
curl -X POST localhost:8080/v1/auth/magic-links -H 'content-type: application/json' \
  -d '{"email":"you@example.com"}'
# copy the token after #token= from the log, then:
curl -i -X POST localhost:8080/v1/auth/magic-link-redemptions \
  -H 'content-type: application/json' -d '{"token":"…"}'
```

In a deployment, the CNPG database owner (with `CREATEROLE`, never a
superuser) runs `glossa-server -migrate=only` as a Job. `glossa_app` gets
its login and password from a CNPG managed role with a `passwordSecret`.
The server refuses to start if `DATABASE_URL` is a superuser or
`BYPASSRLS` role, or if that role can't `SET ROLE glossa_system`.

### Environment

| Variable | Default | Meaning |
|---|---|---|
| `DATABASE_URL` | required | App role DSN. Pool sizing goes in the DSN (`pool_max_conns=20`). |
| `MIGRATION_DATABASE_URL` | — | Owner DSN. Required when migrating. |
| `GLOSSA_MIGRATE` | `off` | `off`, `up` (migrate, then serve) or `only` (migrate, then exit). The `-migrate` flag overrides it. |
| `GLOSSA_HTTP_ADDR` | `:8080` | Listen address. |
| `GLOSSA_HTTP_READ_HEADER_TIMEOUT` / `_READ_TIMEOUT` / `_WRITE_TIMEOUT` / `_IDLE_TIMEOUT` | `5s` / `30s` / `30s` / `120s` | Server timeouts. |
| `GLOSSA_HTTP_MAX_BODY_BYTES` | `1048576` | Request body cap. |
| `GLOSSA_LOG_LEVEL` | `info` | `debug`, `info`, `warn` or `error`. |
| `GLOSSA_SHUTDOWN_TIMEOUT` | `25s` | Drain budget after SIGTERM. Keep it below the pod's grace period. |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | — | Turns on OTLP/HTTP trace export. The other `OTEL_*` variables apply. |
| `OTEL_SERVICE_NAME` | `glossa-server` | Trace resource name. |
| `GLOSSA_OUTBOX_ENABLED` | `true` | Run the dispatcher in this process. |
| `GLOSSA_OUTBOX_POLL_INTERVAL` | `1s` | Idle poll interval. |
| `GLOSSA_OUTBOX_BATCH_SIZE` | `50` | Events claimed per round trip. |
| `GLOSSA_OUTBOX_MAX_ATTEMPTS` | `10` | Deliveries before an event is dead-lettered. |
| `GLOSSA_OUTBOX_LEASE` | `1m` | Claim lease. Must exceed the handler timeout. |
| `GLOSSA_OUTBOX_HANDLER_TIMEOUT` | `30s` | Budget for one subscriber, retries included. |
| `GLOSSA_AUTH_SECRET` | required | Base64 of ≥ 32 random bytes. The CSRF, TOTP-sealing and passkey-state keys are derived from it (HKDF). Rotating it invalidates CSRF tokens and in-flight passkey ceremonies and makes enrolled TOTP secrets unreadable. |
| `GLOSSA_STUDIO_URL` | `http://localhost:5173` | Studio's origin; emailed links point into it. |
| `GLOSSA_SESSION_TTL` | `336h` | Session lifetime. |
| `GLOSSA_MAIL_DRIVER` | `log` | `log` (development: mail goes to the log, links included) or `smtp`. |
| `GLOSSA_MAIL_FROM` | `Glossa <no-reply@localhost>` | Sender. |
| `GLOSSA_SMTP_ADDR` / `_USERNAME` / `_PASSWORD` | — | Submission server (`host:587`) and AUTH PLAIN credentials. STARTTLS is required. |
| `GLOSSA_SMTP_ALLOW_PLAINTEXT` | `false` | Allow a server without STARTTLS (a local relay only). |
| `GLOSSA_WEBAUTHN_RP_ID` | — | Passkey relying party ID (registrable domain). Passkeys are off while unset. |
| `GLOSSA_WEBAUTHN_RP_NAME` | `Glossa` | Name shown by authenticators. |
| `GLOSSA_WEBAUTHN_ORIGINS` | `GLOSSA_STUDIO_URL` | Comma-separated origins allowed to run passkey ceremonies. |

### Tests

```sh
go test -race ./...                            # unit (incl. contract lint and generated-code freshness)
go test -tags=integration -timeout=300s ./...  # Docker: Postgres 16 via testcontainers
go generate ./db/...                           # regenerate sqlc code (sqlc pinned in db/generate.go)
go generate ./internal/apiv1/...               # regenerate the /v1 server (oapi-codegen pinned as a go.mod tool)
```

The integration harness (`internal/kernel/db/dbtest`) provisions the
database the way production does: a `CREATEROLE` owner runs the
migrations, and the tests connect as `glossa_app`.

## Contract for context authors

### Tenancy and row-level security

- Every tenant-owned table has a `tenant_id uuid NOT NULL` column plus
  this block in its migration. The RLS guard test (`TestRLSGuard`) reads
  the catalog and fails CI for any table that doesn't. A table without
  tenant data that must be reached before a tenant is known (Identity's
  people, sessions, credentials) goes in the guard's `systemTables` with
  a reason: it still forces RLS, is granted to `glossa_system` only, and
  `glossa_app` may read it only through a `PUBLIC … FOR SELECT` policy
  scoped by `app_current_tenant()`. `globalTables` (outside RLS
  entirely) is for bookkeeping like `schema_migrations`.

  ```sql
  ALTER TABLE things ENABLE ROW LEVEL SECURITY;
  ALTER TABLE things FORCE ROW LEVEL SECURITY;
  CREATE POLICY things_tenant_isolation ON things
      USING (tenant_id = app_current_tenant())
      WITH CHECK (tenant_id = app_current_tenant());
  GRANT SELECT, INSERT, UPDATE, DELETE ON things TO glossa_app;  -- only what you need
  ```

- Every query runs inside `uow.InTenantTx(ctx, fn)`. The tenant comes
  from `ctx` and nowhere else. It's put there by `tenancy.Middleware`,
  whose `Resolver` (Identity) takes the `{tenant}` of
  `/v1/tenants/{tenant}/…` and accepts it only if the session's person
  is an active member or the bearer token belongs to it — never from a
  header. Pass the `*db.TenantTx` to your sqlc `New(tx)`. The
  transaction commits when `fn` returns nil. Nested units of work are
  refused.
- `uow.InSystemTx(ctx, db.NewSystemScope("ctx.job"), fn)` is for
  tenantless background work only. It refuses a context that carries a
  tenant, and it runs as `glossa_system`, which sees nothing unless a
  migration grants it a table and adds a `TO glossa_system` policy.
  That policy must then be listed in the guard's `systemPolicies`.

### The /v1 API

`api/openapi.yaml` is the contract; its description lists the
conventions every operation follows (`/v1` + kebab-case resources,
tenant-owned resources under `/v1/tenants/{tenant}`, cursor pagination,
problem codes, `ETag`/`If-Match`, `Idempotency-Key`, RFC 3339 UTC, opaque
string IDs, the two security schemes). `api/openapi_test.go` validates
the document and enforces those conventions per operation, so an
operation that skips one fails CI.

To add operations: write them in the spec, run
`go generate ./internal/apiv1/...` (`TestGeneratedCodeIsCurrent` fails
on a stale `apiv1.gen.go`), implement the new methods on your context's
handler type in `internal/<context>/adapters/httpapi`, and embed that
type in `apiServer` (`cmd/glossa-server/api.go`, through an alias: every
context names its type `API`; `var _ apiv1.StrictServerInterface =
apiServer{}` fails the build until every operation has a handler). You don't write auth
code: Identity's `Guard` reads each route's `security` from the
contract and enforces it, and on `/v1/tenants/{tenant}/…` your handler
runs with the tenant and principal on its context. Authorize in the
application layer:

```go
if err := authz.Require(ctx, authz.CatalogWrite); err != nil { return err }       // 403
if err := authz.RequireFor(ctx, authz.TranslationsWrite, locale); err != nil { … } // locale-scoped
```

Return errors, not responses: a `*problem.Details` (with a snake_case
code listed in the spec) passes through as is; Identity's mapping covers
authz and its own errors, and anything unknown is a logged 500.
Paginate with `pagination.Parse` / `pagination.Trim`. The TypeScript
client for Studio will be generated from the same spec.

### Domain events

Event types are `<context>.<aggregate>.<past-tense verb>` in snake_case —
`identity.member.added`, `catalog.message.source_revised`. The middle
segment is the event's `AggregateType`, and `AggregateID` is the
aggregate's ID. Payloads are JSON objects with snake_case keys that
carry IDs and the facts of the change, never secrets. A released name
never changes meaning: a breaking payload change is a new type with a
`.v2` suffix, published alongside the old one until its subscribers
move. Identity publishes `identity.tenant.created`,
`identity.member.{added,activated,access_changed,removed}` and
`identity.token.{created,revoked}`. Catalog and Localization's events
are listed under their sections below.

### Outbox

```go
err := uow.InTenantTx(ctx, func(ctx context.Context, tx *db.TenantTx) error {
    // … change state with sqlc on tx …
    _, err := outbox.Publish(ctx, tx, outbox.Event{
        Type: "catalog.message.source_revised", AggregateType: "message", AggregateID: id,
        Payload: SourceRevised{Revision: rev},
    })
    return err
})

events.Subscribe("catalog.message.source_revised", "localization.mark_outdated",
    outbox.HandlerFunc(func(ctx context.Context, d outbox.Delivery) error {
        var e SourceRevised
        if err := d.Decode(&e); err != nil { return err } // already Permanent
        return uow.InTenantTx(ctx, markOutdated(d.EventID, e)) // ctx is tenant-scoped
    }))
```

- **Delivery is at least once.** A handler must be idempotent on
  `d.EventID`, for example by recording processed IDs in its own
  transaction or by upserting.
- If the state change rolls back, the event was never published.
- Each subscriber is retried on its own. A subscriber that already
  succeeded isn't called again. Keep subscriber names stable, because
  they're stored with the event.
- A failing handler is retried in process, then rescheduled with
  exponential backoff. After `GLOSSA_OUTBOX_MAX_ATTEMPTS` deliveries the
  event is dead-lettered (`status = 'dead'`). Wrap an error in
  `outbox.Permanent` to dead-letter it immediately.
- Handlers run under the publisher's trace and the event's tenant.
  There's no ordering guarantee.

## Identity

People, their credentials and sessions are global to the deployment; a
person belongs to their `individual` tenant (created at registration)
and to any number of `organization`s. Memberships and API tokens are
tenant-owned.

| Table | Scope | Why |
|---|---|---|
| `identity_people` | system | A person signs in before choosing a tenant and spans many. Tenant scope reads only the id, email and name of the current tenant's members. |
| `identity_sessions`, `identity_email_links`, `identity_totp`, `identity_passkeys`, `identity_webauthn_ceremonies`, `identity_login_attempts` | system | auth-go's state, all consulted before a tenant exists. Tokens and links are stored as SHA-256 hashes, TOTP secrets AES-256-GCM sealed. |
| `identity_members` | tenant | A person's roles and locales in one tenant. `glossa_system` may SELECT (validating `/v1/tenants/{tenant}`, listing a person's tenants, finding invitations). |
| `identity_api_tokens` | tenant | `glossa_system` may SELECT by hash and bump `last_used_at` (resolving a bearer token's tenant). |

**Sign-in** uses auth-go's domain services (`SessionService`,
`MagicLinkService`, `TOTPService`, `LockoutService`, the WebAuthn
adapter) over Glossa's Postgres adapters: magic link (15 min, creates the
account on first use, verifies the address), password + optional TOTP
(Argon2id, lockout after 5 failures for 15 min), passkeys (ceremony
state server-side, single-use). Registration writes the person, then the
individual tenant and owner membership in one tenant transaction; every
sign-in repairs a missing individual tenant and accepts open invitations
to the verified address. Password reset revokes all sessions.

**Roles** — `owner`, `admin`, `developer`, `translator`, `reviewer`; the
matrix is pinned by `TestRolePermissionMatrix`. Translators and
reviewers can be limited to canonical BCP 47 locales, which cover their
CLDR descendants (`de` covers `de-AT`). **Token scopes** — `read`,
`write`, `publish`, `admin`; every scope implies read, none grants
review or owner changes, and a token never exceeds its creator.

**API tokens** look like `glossa_api_` + 43 base64url characters.
Register `glossa_api_[A-Za-z0-9_-]{43}` with secret scanners. They're
shown once, stored as SHA-256, revocable, optionally expiring, and track
`last_used_at` (at most one write a minute). The publishable delivery key
of M1 will get its own prefix and table.

### auth-go follow-ups

Gaps found while integrating auth-go v0.7.2. Glossa doesn't work around
them beyond implementing auth-go's own ports; each belongs in auth-go:

1. **Global principals.** `User`, `Session` and `MagicLink` each belong to
   one `TenantID`. Glossa's people are global and pick a tenant per
   request, so every auth-go object carries a fixed realm (`glossa`).
   auth-go could make the tenant optional or call it a realm.
2. **Link purpose.** `MagicLink` has no purpose, so sign-in and password
   reset links are kept apart by giving each purpose its own repository.
   A purpose (or audience) on the link would do it in the domain.
3. **Pending TOTP enrollment.** `TOTPRepository` has one secret per user
   and no pending/confirmed state; Glossa tracks confirmation itself.
4. **Server-side WebAuthn ceremonies.** auth-go returns ceremony state to
   the caller and recommends server-side storage; a `CeremonyStore` port
   with single-use `Take` would make that the default.
5. **Discoverable-credential (usernameless) passkey sign-in.**
   `BeginLogin` needs a user ID, so passkey sign-in starts from an email,
   which tells a caller whether that address has passkeys.
6. **Product API tokens.** `WorkloadKeyService` fixes the token format
   (64 hex, no prefix, so no secret scanning), requires an expiry, is not
   tenant-bound, deletes on revoke (no audit trail) and has no last-used
   tracking. Glossa's tokens reuse auth-go's `NewToken` and `HashToken`
   and own the rest; a prefixed, tenant-bound key type in auth-go would
   let products share it.
7. **pgx.** `pgstore` is `database/sql` with its own schema and no RLS
   story; products on pgx + RLS (Glossa) implement every port themselves.
8. **Password policy.** No length/strength helper beyond the 1024-byte cap;
   Glossa enforces a 12-character minimum.

## Catalog

What the product says (RFC 0002 §4). A **project** (tenant-owned, slug,
fixed source locale, settings) has **applications** (web, api, ios,
android, other) and **messages**. A message has an immutable ID and a
key runtimes use (`checkout.pay`, unique per project); its source is
stored as the canonical MF2 data model plus the authored text and syntax
(ICU MF1 by default), parsed only by the `messageformat` kernel.
Arguments and markup are derived from the model on every change. Only a
model change is a new source revision, so pushing the same catalog
twice, in either syntax, changes nothing. Renaming is explicit and keeps
the ID and all history.

| Table | Scope | Why |
|---|---|---|
| `catalog_projects`, `catalog_applications`, `catalog_messages` | tenant | Ordinary state; the message row is the projection of its latest source revision. |
| `catalog_source_revisions` | tenant | Append-only by grant (`glossa_app`: SELECT, INSERT). History is the domain here (RFC 0002 §4). |

Events: `catalog.project.{created,updated,deleted}`,
`catalog.application.{created,updated,deleted}`,
`catalog.message.{created,source_revised,updated,renamed,obsoleted,reactivated}`.
Every message event carries a `message` snapshot (`message_id`,
`project_id`, `key`, `namespace`, `state`, `source_revision`, `version`);
`source_revised` adds `old_revision` and `new_revision`, `renamed` adds
`old_key` and `new_key`. A consumer keeps the snapshot with the highest
`version`, which makes delivery order and duplicates irrelevant.

Permissions: reads need `catalog.read`, writes `catalog.write`
(developers, admins, owners, `write` tokens), deleting a project
`tenant.manage`. `POST …/message-upserts` is the CLI's push: up to 500
items in one transaction, per-item results, `base_revision` to refuse
overwriting a newer revision, obsolete keys reactivated.

## Localization

What each locale says. **Locales** are canonical BCP 47 (no extensions
or private use, ≤ 35 characters) with a direction derived from the likely
script; the source locale is one of them from the project's creation.
The **fallback graph** is exactly the manifest's `fallback` object,
validated against the project's locales and refused if cyclic. A
**translation** (message × locale) is the projection of an append-only
**revision** log; every revision carries provenance (`origin`:
human, ai, translation_memory, machine_translation, import, adaptation;
`origin_detail`; the principal) and the source revision it was made
against. Outdated is derived — `source_revision <` the message's current
one — and never stored. Every write is checked with
`messageformat.CheckCompat` against the source revision it claims: error
findings reject it (`422 structural_qa_failed` with `findings`),
warnings (and `max-length-exceeded`) are stored and returned. Review
state (`draft`, `needs_review`, `approved`, `rejected`) is data: a
`ReviewFlow` of allowed transitions and reviewer-only states, plus the
project's `review_required` setting, for the workflow engine to replace.

| Table | Scope | Why |
|---|---|---|
| `localization_locales`, `localization_fallback_graphs` | tenant | Ordinary state keyed by Catalog's project ID (no cross-context foreign keys). |
| `localization_messages` | tenant | Localization's own projection of Catalog messages, fed by `catalog.message.*` (highest `version` wins) and refreshed synchronously on every translation write. It is what "outdated" and the `missing_in`/`outdated_in` filters read. |
| `localization_translations` | tenant | Projection of the latest revision. |
| `localization_translation_revisions` | tenant | Append-only by grant. |

Localization reads Catalog only through Catalog's application service
(`adapters/catalog`, the `SourceCatalog` port) and answers Catalog's
coverage filter through `adapters/coverage`; neither context touches
the other's tables. Subscribers (names are stored with events; never
rename them): `localization.track_message` on every
`catalog.message.*`, `localization.add_source_locale` on
`catalog.project.created`, `localization.drop_project` on
`catalog.project.deleted`. Events: `localization.locale.{added,removed}`,
`localization.fallback_graph.changed`,
`localization.translation.{revised,reviewed}`, and
`localization.translation.outdated` — once per translation a source
revision leaves behind, the trigger for re-translation (M2) and review
routing.

Permissions: reads need `translations.read`; locales and the fallback
graph `catalog.write`; writing a translation `translations.write` for
its locale (translators and reviewers are limited to their locale scope,
which covers CLDR descendants); approving or rejecting
`translations.review` for the locale. No token scope grants review, so
with `review_required` a token's writes wait for a human.

### What Release reads

A release is built from two application reads, each one tenant
transaction, joined by message ID:

- `catalogapp.Service.ReleaseSource(ctx, project)` → the project (source
  locale) and every **active** message in key order: key, namespace,
  current source content (the source locale's artifact).
- `localizationapp.Service.ReleaseTranslations(ctx, project, states)` →
  the locales with direction (source first), the fallback graph as the
  manifest's `fallback`, and per locale the translations in the eligible
  review states (production: `approved`), each with its canonical model,
  `source_revision` and derived outdated flag.

Release keeps translations of active messages in the project's locales
and writes one artifact per locale and namespace (runtimes/SPEC.md §1).
