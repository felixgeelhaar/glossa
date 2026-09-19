# platform

Go module for Glossa's control and delivery planes (RFC 0002). It holds
the `glossa-server` kernel (configuration, observability, the HTTP edge,
Postgres with forced row-level security, tenancy, the transactional
outbox), the `/v1` API contract, the bounded contexts under
`internal/<context>/` (Identity, Catalog, Localization, Release and
Knowledge so far) and `glossa-edge`, the stateless delivery server.

```text
api/openapi.yaml            the /v1 contract (OpenAPI 3.1), source of truth
cmd/glossa-server/          composition root only (plus GET /v1/meta, which gathers deployment facts)
cmd/glossa-edge/            composition root of the delivery plane (no database)
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
  jcs/                      RFC 8785 canonical JSON (artifacts, manifests, signatures)
  objectstore/              object storage port; dir, memory and S3 (minio-go) adapters
internal/apiv1/apiconv/     message content, QA findings and If-Match on the wire
internal/catalog/           projects, applications, messages, source revisions
internal/localization/      locales, fallback graphs, translations, revisions
internal/release/           environments, releases, artifacts, signing, delivery keys
  delivery/                 the bucket layout and key format glossa-edge shares
internal/knowledge/         translation memory, termbase, style guides (RFC 0003 §2)
  domain/                   TM normalization and derivation, term recognition, terminology QA, style merge
  app/                      use cases, the outbox subscribers, Reader (the port Intelligence consumes)
  adapters/                 postgres (sqlc), sources (Catalog/Localization ports), httpapi
internal/preview/           stateless message preview (parse, MF2, format), rate-limited per caller
internal/edge/              glossa-edge's handler and server (object storage only)
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

By default the server sends no email (`GLOSSA_MAIL_DRIVER=none`):
register with a password and sign in with it (see *Identity*, "Without
email"). For magic links in development, ask for the log mailer, which
writes every mail, links included, to the log:

```sh
GLOSSA_MAIL_DRIVER=log go run ./cmd/glossa-server
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
| `GLOSSA_MAIL_DRIVER` | `none` | `none` (no email: magic links and password reset by email are off, password accounts work unverified), `smtp`, or `log` (development only: mail goes to the log, links included). |
| `GLOSSA_MAIL_FROM` | `Glossa <no-reply@localhost>` | Sender. |
| `GLOSSA_SMTP_ADDR` / `_USERNAME` / `_PASSWORD` | — | Submission server (`host:587`) and AUTH PLAIN credentials. STARTTLS is required. |
| `GLOSSA_SMTP_ALLOW_PLAINTEXT` | `false` | Allow a server without STARTTLS (a local relay only). |
| `GLOSSA_WEBAUTHN_RP_ID` | — | Passkey relying party ID (registrable domain). Passkeys are off while unset. |
| `GLOSSA_WEBAUTHN_RP_NAME` | `Glossa` | Name shown by authenticators. |
| `GLOSSA_WEBAUTHN_ORIGINS` | `GLOSSA_STUDIO_URL` | Comma-separated origins allowed to run passkey ceremonies. |
| `GLOSSA_STORAGE_DRIVER` | `dir` | Object storage for release artifacts: `dir` (a local directory, single node) or `s3`. glossa-edge reads the same storage. |
| `GLOSSA_STORAGE_DIR` | `data/objects` | The directory for `dir`. |
| `GLOSSA_S3_ENDPOINT` / `_BUCKET` / `_REGION` / `_PREFIX` | — / — / `us-east-1` / — | S3-compatible bucket: `host[:port]` without a scheme; the prefix lets deployments share a bucket. |
| `GLOSSA_S3_ACCESS_KEY_ID` / `_SECRET_ACCESS_KEY` | — | Credentials. glossa-edge needs read access only. |
| `GLOSSA_S3_PATH_STYLE` / `_INSECURE` / `_TIMEOUT` | `false` / `false` / `10s` | Path-style requests (MinIO), plain HTTP (local only), per-operation budget. |
| `GLOSSA_RELEASE_SIGNING_KEYS` | derived | `keyId=base64(32-byte Ed25519 seed)`, comma-separated. Every manifest is signed with each. Unset: one key derived from `GLOSSA_AUTH_SECRET` (development only; a warning is logged). |
| `GLOSSA_RELEASE_RETIRED_KEYS` | — | `keyId=base64(public key)`, comma-separated: still published for verification, no longer signing. |
| `GLOSSA_EDGE_PUBLIC_URL` | — | glossa-edge's public base URL (`https://edge.example.com`). `GET /v1/meta` announces it, so Studio's snippets and other clients don't guess. |

`glossa-edge` reads `GLOSSA_HTTP_*` (listening on `:8081` by default),
`GLOSSA_LOG_LEVEL`, `GLOSSA_SHUTDOWN_TIMEOUT`, `OTEL_*` (service
`glossa-edge`), the `GLOSSA_STORAGE_*`/`GLOSSA_S3_*` variables above, and:

| Variable | Default | Meaning |
|---|---|---|
| `GLOSSA_EDGE_CACHE_BYTES` | `67108864` | In-process cache for manifests and artifacts. |
| `GLOSSA_EDGE_KEY_TTL` | `30s` | How long a key resolution is trusted: the longest a revoked key keeps working at one edge. |
| `GLOSSA_EDGE_MANIFEST_TTL` | `5s` | How long a manifest is served before storage is asked again: the delay of a publish, promote or rollback at one edge (a CDN adds its 60 s max-age). |

```sh
GLOSSA_STORAGE_DIR=data/objects go run ./cmd/glossa-edge   # beside a local glossa-server
```

### Tests

```sh
go test -race ./...                            # unit (incl. contract lint and generated-code freshness)
go test -tags=integration -timeout=300s ./...  # Docker: Postgres 16 via testcontainers
go generate ./db/...                           # regenerate sqlc code (sqlc pinned in db/generate.go)
go generate ./internal/apiv1/...               # regenerate the /v1 server (oapi-codegen pinned as a go.mod tool)
go generate ./internal/apiclient/...           # regenerate the CLI's /v1 client from the same spec
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
- A handler has a tenant but no principal. To read (or write) another
  context through its application service — which checks permissions
  like any use case — act as a named background principal with exactly
  the permissions it needs:

  ```go
  bg, err := authz.Background(ctx, "knowledge.derive_tm", authz.TranslationsRead, authz.CatalogRead)
  ```

  Its actor is `system:<uuid>` (stable per name). It refuses a context
  that already carries a principal and never grants identity
  administration (`tenant.manage`, `members.manage`, `owners.manage`,
  `tokens.manage`).

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
`GET /v1/me/passkeys` lists the person's passkeys from every device
(name, created, last used; oldest first) and `DELETE
/v1/me/passkeys/{id}` removes one — scoped to the signed-in person in
the query itself, so another person's passkey is a `404`. Both work
while passkeys are not configured, so enrolled ones stay manageable.

**Without email** (`GLOSSA_MAIL_DRIVER=none`, the default; SMTP is
optional for a first install) the email flows are unavailable rather
than silently dropped: requesting or redeeming a magic link and
requesting or redeeming a password reset answer `404 email_disabled`.
Registration creates the account with its password, unverified, and
signing in works at once — password (+ TOTP) and passkeys, with no
verification step. `GET /v1/meta` tells clients which methods exist
(`sign_in_methods`: `passkey` when a relying party is configured,
`password`, `magic_link` only with email; `email_delivery`), and Studio
hides what isn't there. The trade-offs, accepted until SMTP is
configured:

- An address is a claim, not a proof. Anyone can register any address
  first, and its owner then can't register it or reset its password.
- Nobody can reset a forgotten password (there is no admin reset yet);
  enroll a passkey or TOTP and keep the password safe.
- Invitations wait: only a verified address accepts one (so a stranger
  who registers an invited address can't join the organization), and
  without email no address is verified. Configure SMTP before inviting
  people, or they join once it is and they sign in with a magic link.
- Registration answers `202` either way, but registering then signing in
  tells a caller whether someone else already holds the address.

Switching email on later changes nothing retroactively: unverified
accounts then need their address verified (a magic link) before
password sign-in works again. The log mailer (`log`) is for development
only and must be asked for.

**Roles** — `owner`, `admin`, `developer`, `translator`, `reviewer`; the
matrix is pinned by `TestRolePermissionMatrix`. Translators and
reviewers can be limited to canonical BCP 47 locales, which cover their
CLDR descendants (`de` covers `de-AT`). **Token scopes** — `read`,
`write`, `publish`, `admin`; every scope implies read, none grants
review or owner changes, and a token never exceeds its creator.
Every role and the `read` scope hold `knowledge.read`;
`knowledge.write` (curating the termbase, style guides and TM) belongs
to owners, admins, developers and `write` tokens. The AI permissions:
every role and the `read` scope hold `intelligence.read` (configuration
without keys, jobs, suggestions, disclosures, metrics);
`intelligence.manage` (providers, routing, prices, budget, consent,
sensitive namespaces, auto-translate, review routing) belongs to owners,
admins and `admin` tokens; `intelligence.translate` (request fills,
cancel jobs, accept, edit or reject suggestions) is **locale-scoped**
like `translations.write` and belongs to every role but none of the
read-only ones — translators and reviewers within their locales — and
to `write` tokens.

**API tokens** look like `glossa_api_` + 43 base64url characters.
Register `glossa_api_[A-Za-z0-9_-]{43}` with secret scanners. They're
shown once, stored as SHA-256, revocable, optionally expiring, and track
`last_used_at` (at most one write a minute). Publishable delivery keys
(`glossa_pk_…`) are Release's, below.

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

**Bulk reads.** `GET …/projects/{project}/translations?locale=de&locale=fr`
lists translations across messages for 1–20 locales, ordered by
message key, message ID and locale, each with the message's `key`,
`namespace` and `message_state`, its `source_revision` and the derived
`outdated`; filters: `state` (repeatable), `outdated`, `namespace`,
`key_prefix`, `message_state`. It is one query per page: keyset
pagination on `(key, message_id, locale)`, driven by the
`localization_messages_project_order` index (0006; `(project_id, key,
message_id) INCLUDE (namespace, state, source_revision)`) and the
translations' `UNIQUE (message_id, locale)`, so a page costs the same
at any depth. Translations of removed locales are not listed.
`GET …/projects/{project}/translation-stats` summarizes every locale
in one query: active messages, `translated` (a usable — not rejected —
translation), `missing` (none or rejected), `outdated` (usable, older
source revision) and translations of active messages per review state;
the source locale counts as fully translated and approved. It is an
aggregate computed per request rather than counters kept on every
write: one source revision changes every locale's outdated count, and a
single grouped pass over the project's rows (about 0.1 s for 100 000
translations) is correct by construction.

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

## Knowledge

The organization's linguistic knowledge (RFC 0003 §2, intent §19):
translation memory, the termbase and style guides — separate
aggregates, never one "AI context" blob. Everything is tenant-owned and
either tenant-wide or scoped to one project.

| Table | Scope | Why |
|---|---|---|
| `knowledge_tm_units` | tenant | TM units, active and retired (history); at most one active unit per translation. |
| `knowledge_tm_derivations` | tenant | The translation revision each translation's units reflect: the idempotency guard of derivation. |
| `knowledge_concepts`, `knowledge_terms` | tenant | The termbase; terms are replaced with their concept. |
| `knowledge_concept_revisions` | tenant | Full snapshots per version, outliving the concept. SELECT, INSERT (DELETE only to erase a deleted project). |
| `knowledge_style_guides` | tenant | One guide per scope (`NULLS NOT DISTINCT` unique index). |
| `knowledge_style_guide_versions` | tenant | Full snapshots per version, outliving the guide; same grants as concept revisions. |

**pg_trgm.** Migration 0007 runs `CREATE EXTENSION IF NOT EXISTS
pg_trgm` for fuzzy matching and substring search (GIN trigram indexes).
It ships with PostgreSQL's contrib modules — in the official images
(the `postgres:16-alpine` test container) and CloudNativePG's operand
images — and is a trusted extension, so the CNPG database owner that
runs `-migrate=only` creates it without superuser rights. A cluster
built without contrib must provide it first.

**Translation memory.** Units are derived, not curated:
`knowledge.derive_tm` subscribes to `localization.translation.revised`
and `.reviewed`, reads the translation's *current* state through
Localization's `TranslationWithSource` (as the background principal
`knowledge.derive_tm` with `translations.read` and `catalog.read`) and
reconciles (`domain.Reconcile`): an approval creates a unit or keeps the
one with the same text; other approved text supersedes it; losing the
approval retires it (`unapproved`), unapproved new text too
(`overwritten`). Units are retired, never deleted. Under a row lock, a
state no newer than the revision already applied changes nothing, so
duplicates and reordering converge (`TestDerivationIsIdempotentAndOrderIndependent`
replays every event newest first).

A unit stores both sides as canonical MF2, the target's data model, and
the **normalized** source: pattern text with placeholders by position
(`{$amount}` → `{1}`, `.local`s resolved to their argument), markup as
tags (`<b>`, `</b>`, `<br/>`), whitespace collapsed, NFC; a select
message becomes `.match {1}` plus one `keys {pattern}` line per variant.
The **signature** lists the placeholders' types (`1:number/plural`).
Lookups (`LookupTM`, `POST …/tm-lookups`) score:

- **101** — exact, approved for the same message key in the same
  namespace of the same project (the context match);
- **100** — same normalized text (SHA-256) and signature;
- **50–99** — `pg_trgm` similarity of the normalized text, floored to
  a percentage and capped at 99 (identical text with other placeholder
  types scores 99). `min_score` sets the similarity threshold of the
  `%` operator, so the trigram index does the filtering.

A lookup sees tenant-wide units and the query's project's
(`all_projects` widens it to the tenant); ties rank the own project
first, then the most recently confirmed unit, and each target appears
once. Targets come back with their variables renamed to the query's
(`domain.AdaptVariables`), so `Pay {$total}` reuses `{$amount} zahlen`
as `{$total} zahlen`. `TestFuzzyMatchingQuality` pins quality on a
seeded 40-unit en→de memory: 30 near-misses (rewording, plurals,
renamed variables) find the intended unit first 30/30 times, and 0 of 8
unrelated messages match at all. Concordance (`GET …/tm-concordance`)
is an `ILIKE` substring search over either normalized side, ranked by
`word_similarity`. Semantic (vector) matching is a later port (needs
pgvector).

**Termbase.** A concept (definition, domain, note, product reference)
owns its terms: locale, text, status (`preferred`, `admitted`,
`deprecated`, `forbidden`), part of speech, case sensitivity, note.
Replacing it keeps the IDs of terms that stay; every version is a
snapshot. A term applies to its locale and that locale's descendants.
**Recognition** (`domain.Termbase.Recognize`) is deterministic and
dictionary-free: words are runs of letters, marks and digits (hyphens,
apostrophes and script changes separate them); case is folded per rune
(Turkish/Azerbaijani i) unless the term is case-sensitive; a term
matches whole words from a word start, each word allowing a short
inflectional ending (≤ 3 letters from 5-letter words, ≤ 2 for 4-letter
words, none below; Hangul ≤ 3 syllables from 2) — so *workspaces*,
*Rechnungen*, *cartes bancaires* match, but not *Konten* or compounds.
Terms in Han, Hiragana, Katakana, Thai, Lao, Khmer, Myanmar or Tibetan
script match as **substrings**, because those texts have no spaces; a
short term can then match inside a longer word (会議 in 会議室).
Overlaps resolve leftmost-longest; homonyms on one span are all kept.
**Terminology QA** (`domain.CheckTerminology`): `term_missing`
(warning) when a concept found in the source has none of its preferred
or admitted target terms in the translation; `term_forbidden` (error;
warning for deprecated terms) for each forbidden or deprecated target
term — unless the same words are an allowed term of a concept the
source mentions. Messages are checked as `domain.VisibleText`
(placeholders become U+FFFC, so `{$workspace}` is not a word). The API
reports code point offsets.

**Style guides** are structured: formality (register + pronoun), tone
tags, punctuation (quotes, dash, spaces before units and punctuation,
serial comma, ellipsis), number and date conventions, and rules with
rationale and good/bad examples. A scope is any combination of project,
locale and namespace (a namespace needs a project). The **effective**
style (`EffectiveStyle`, `GET …/effective-style-guide`) merges the
applicable guides leaf by leaf, the narrowest last — namespace beats
locale, a deeper locale a shallower one, locale beats project, project
beats tenant — a narrower rule replaces a broader one with the same
`id` or disables it, and `sources` names each guide version used, for
provenance.

**The Intelligence read port** (`app.Reader`, implemented by
`*app.Service`), free of HTTP types; each call is authorized with
`knowledge.read` in the tenant on `ctx` (a job acts through
`authz.Background(ctx, name, authz.KnowledgeRead, …)`):

```go
type Reader interface {
	LookupTM(ctx context.Context, q TMQuery) ([]TMMatch, error)                         // tm_lookup
	RecognizeTerms(ctx context.Context, q TermQuery) ([]RecognizedTerm, error)          // term_lookup
	CheckTerminology(ctx context.Context, c TermCheck) ([]domain.TermFinding, error)    // validate
	EffectiveStyle(ctx context.Context, q StyleQuery) (domain.EffectiveStyle, error)   // style_rules
}
```

Events: `knowledge.concept.{created,updated,deleted}`,
`knowledge.style_guide.{created,updated,deleted}` (IDs, scope, version,
actor; TM units are derived state and publish none). Subscribers:
`knowledge.derive_tm` (above), `knowledge.drop_project` on
`catalog.project.deleted` (erases the project's units, concepts, guides
and their history). Permissions: reads, lookups, recognition and checks
need `knowledge.read`; creating, replacing, deleting and retiring
`knowledge.write`.

## Message preview

`POST /v1/message-previews` runs the MessageFormat kernel — the one MF1
converter (RFC 0002 §5) — for clients that aren't Go: Studio's live MF1
preview, and the same results the CLI's offline checks compute
in-process. It takes `{source, syntax (mf1|mf2, default mf1), locale,
values?, bidi_isolation?}` and returns `{valid, message (canonical MF2
data model), mf2, arguments, markup, formatted? (with values), errors}`.
Source the kernel rejects is a `200` with `valid: false` and
`errors[].stage = parse` (codes like `mf1-syntax-error`); formatting
problems are `stage = format` with MF2's fallback text in `formatted`.
Caller mistakes are `400`: `message_too_long` (> 20 000 bytes),
`invalid_locale`, `invalid_syntax`, `invalid_values` (> 100 values,
names > 64 characters, strings > 1 000 bytes, or anything but a
string, number or boolean).

`internal/preview` has no domain state and no tables: `app` checks the
caller with `authz.Authenticated` (any person or API token; no tenant
data, so no permission) and asks its `Limiter` port, which
`adapters/ratelimit` implements with a fortify token bucket per
principal (`person:<id>` / `token:<id>`): **10 a second, bursts of
120**, in process — each instance enforces it on its own, which bounds
the CPU one caller can take from any instance; idle buckets expire
after 10 minutes, at most 100 000 are tracked. Over the limit is `429
rate_limited`.

## Release

What ships where (RFC 0002 §7, intent §34–38). Every project has the
environments `development`, `preview`, `staging` and `production`
(created on first use) plus any custom ones (`pr-42`; `a` is reserved).
Each has an **eligibility policy** — the review states that ship
(production and staging `approved`; the others `draft`, `needs_review`,
`approved`; never `rejected`) and whether outdated translations do — and
points at the release it serves.

**The path to production** follows from those defaults: `development`
and `preview` (and custom environments like `pr-42`) ship work in
progress; `staging` ships what production would. So publish to
`staging`, check it, and promote that release to `production`, which
moves the pointer and rebuilds nothing. A development or preview release
can never be promoted to production — it was built under a policy that
lets drafts through — and the `release_ineligible` refusal says so,
naming both policies, what differs (`draft, needs_review text`,
`outdated translations`) and the way there (`IneligibleError`).
Publishing to production directly works too.

**Publish** reads Catalog's `ReleaseSource` and Localization's
`ReleaseTranslations` (the policy's states), builds one artifact per
locale and namespace (the source locale holds every active message, a
locale only what is translated for it, `default` always exists), uploads
the artifacts storage doesn't have yet, then in one transaction records
the **release** (version counting the project's releases without gaps,
parent = what the environment served, policy snapshot, author, counts,
manifest digest), points the environment at it and appends the
deployment. A catalog artifacts can't carry (a 64-character
namespace, a malformed model) fails with `422 not_releasable`, listing
every problem found (`NotReleasableError`), not only the first.

**Preview** (`POST …/environments/{env}/release-previews`, the CLI's
`release publish --dry-run`, Studio's publish dialog) runs the same
`build` Publish runs and stops before anything is stored: it returns the
per-locale counts, the manifest digest, `new_artifacts` (what the
publish would upload, from `Exists` checks), the per-locale message IDs
added, changed and removed against the release the environment serves
(the build's artifacts compared in memory, the served release's read
from storage), and the `not_releasable` problems as data rather than an
error. It writes no row, object or event — it doesn't even create the
default environments — which `TestPreviewPublishWritesNothing` checks
against the tables, the outbox and object storage. It needs only
`releases.read`.

**Promote** points an environment at an existing release
whose policy it covers (a preview release with drafts can't reach
production); **rollback** points back to the newest older release the
environment served, or a named one from its history. Both only move the
pointer; nothing is rebuilt.

**Artifacts** are the RFC 8785 canonical JSON of `{schema, locale,
namespace, messages}` with each message's MF2 data model, so equal input
gives byte-identical output whatever order it came in
(`TestBuildIsDeterministic`, `TestArtifactBytesAreCanonical`), and are
addressed by the SHA-256 of those bytes. Publishing an unchanged catalog
uploads nothing, and identical artifacts dedupe across environments.
The **manifest digest** is the SHA-256 of the canonical content every
environment's manifest of the release carries (`sourceLocale`, `locales`,
`fallback`, `artifacts`). **Manifests** are canonical JSON signed with
every active Ed25519 key over the canonical manifest without
`signatures` (SPEC §1.3); both steps are deterministic, so a release
served again (rollback) is served byte for byte as before. Rotation: add
a key to `GLOSSA_RELEASE_SIGNING_KEYS` (manifests carry both
signatures), move runtimes to it (`GET …/release-signing-keys` publishes
the keys), then move the old one to `GLOSSA_RELEASE_RETIRED_KEYS`.

**Storage** is a projection of the database, laid out by
`internal/release/delivery`:

```text
v1/keys/<sha256(key)>.json                               delivery key → project, while the key is active
v1/projects/<project>/environments/<env>/manifest.json   what the environment serves
v1/projects/<project>/a/<sha256>.json                    artifacts, immutable
```

Every change commits first and then writes what it affects, right away
and again from the outbox, under the row's lock and from the row's
current state, so retries and reordering converge and a storage outage
delays delivery without losing a change. Artifacts are addressed per
project: a key only reaches its own project's objects.

| Table | Scope | Why |
|---|---|---|
| `release_releases` | tenant | Immutable: `glossa_app` may SELECT and INSERT; a trigger refuses UPDATE and DELETE for every role except the cascade from erasing the tenant. |
| `release_environments` | tenant | Policy and current release per environment. |
| `release_deployments` | tenant | Every pointer move, append-only by grant: what rollback walks. |
| `release_delivery_keys` | tenant | Publishable keys, in the clear (they ship in bundles); revoked ones stay listed. |

Events: `release.published`, `release.promoted`, `release.rolled_back`
(the Release aggregate shares the context's name, so they are
`release.<verb>`, as SPEC §3 names them),
`release.environment.{created,policy_changed}`,
`release.delivery_key.{created,revoked}` (never carrying the key).
Subscribers: `release.sync_manifest` and `release.sync_delivery_key`
(storage writes), `release.retire_project` on `catalog.project.deleted`
(revokes the project's keys, removes its environments and served
manifests; releases stay as history).

Permissions: reads (and previews) need `releases.read`; publish, promote, rollback,
environment and key changes `releases.publish` (developers, admins,
owners, `publish` tokens).

### glossa-edge

A stateless server that reads object storage and nothing else — a test
fails if its import graph reaches the database, pgx, migrations or any
context's application layer — so published translations keep loading
while glossa-server or Postgres is down (the end-to-end test stops both
and loads a release through the Go runtime). `GET
/v1/{key}/{environment}/manifest.json` answers with `Cache-Control:
public, max-age=60, stale-while-revalidate=300, stale-if-error=86400`, a
strong ETag (the SHA-256 of the bytes) and 304; `GET
/v1/{key}/a/{sha256}.json` is immutable and verified against its hash.
CORS `*` (ETag exposed, preflights answered), never a cookie; unknown or
revoked keys 404, storage failures 503 `no-store`, objects failing their
integrity check 502. An in-process LRU caches keys, manifests and
artifacts with singleflight on misses, and serves stale entries through
a storage outage. `/readyz` ignores storage on purpose, so an outage
can't pull every edge out of the load balancer.
