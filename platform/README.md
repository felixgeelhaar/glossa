# platform

Go module for Glossa's control and delivery planes (RFC 0002). It holds
the `glossa-server` kernel (configuration, observability, the HTTP edge,
Postgres with forced row-level security, tenancy, the transactional
outbox), the `/v1` API contract, the bounded contexts under
`internal/<context>/` (Identity, Catalog, Localization, Release,
Knowledge, Intelligence, Integration and Context so far) and `glossa-edge`, the
stateless delivery server.

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
  sealing/                  AES-256-GCM secrets at rest, bound to their tenant and row
  ratelimit/                in-process token bucket per key (fortify)
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
internal/intelligence/      AI translation (RFC 0003 §3–§4, §7)
  domain/                   provider port, routing, prices, budgets, confidence, config, jobs, suggestions
  app/                      the translation agent and Router; the wiring's Service, subscribers and Worker
  adapters/                 anthropic, openaicompat, gemini, resilient, cassette, memory (library);
                            postgres (sqlc), sources, providers, metrics, httpapi (wiring)
  prompts/, evals/          versioned prompts; golden sets, cassettes and the tracked baseline
internal/integration/       interchange (RFC 0003 §5–§6)
  formats/                  pure converters: XLIFF 2.1, JSON, PO (read), TMX, TBX ↔ the exchange model
  domain/                   import/export jobs, options, access snapshot, state capping, merge conflicts, PO keys
  app/                      import and export use cases, the Worker (claims, checkpoints, retention), mapping
  adapters/                 postgres (sqlc), sources (Catalog/Localization/Knowledge ports), httpapi
internal/context/           where messages appear (RFC 0004 §2–§3)
  domain/                   usages documents, builds, captures and regions, current views, retention
  app/                      ingest, current and unused usages, purge, the outbox subscribers
  adapters/                 postgres (sqlc; the system-scope Sweeper), catalog (Catalog's port),
                            httpapi (the Context API), metrics (Prometheus)
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

In a deployment, the database owner (with `CREATEROLE`, never a
superuser) runs `glossa-server -migrate=only` as a Job, and a later hook
gives `glossa_app` its login and password (see
`deploy/charts/glossa-platform/README.md` for the order).
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
| `GLOSSA_AUTH_SECRET` | required | Base64 of ≥ 32 random bytes. The CSRF, TOTP-sealing, passkey-state and secret-sealing keys are derived from it (HKDF). Rotating it invalidates CSRF tokens and in-flight passkey ceremonies and makes enrolled TOTP secrets and tenants' AI provider keys unreadable (they are entered again). |
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
| `GLOSSA_AI_WORKERS_ENABLED` | `true` | Run AI translation job workers in this process. |
| `GLOSSA_AI_WORKERS` | `2` | Jobs this process runs at once. Tenants' own caps (`max_concurrent_jobs`) apply across replicas. |
| `GLOSSA_AI_POLL_INTERVAL` | `1s` | Idle poll interval of a worker. |
| `GLOSSA_AI_JOB_TIMEOUT` / `_JOB_LEASE` | `10m` / `15m` | Budget for one job (model calls and retries included); how long a claimed job is reserved before another worker takes it over. The lease must be longer. |
| `GLOSSA_AI_PROVIDER_CONCURRENCY` | `4` | Calls in flight per configured provider in this process. |
| `GLOSSA_AI_ALLOW_PRIVATE_ENDPOINTS` | `false` | Let tenants point providers at loopback and private addresses (self-hosted models in the cluster). Off: such base URLs are refused and the provider client won't connect to them. |
| `GLOSSA_INTEGRATION_WORKERS_ENABLED` | `true` | Run import/export job workers (and the retention sweep) in this process. |
| `GLOSSA_INTEGRATION_WORKERS` | `1` | Import/export jobs this process runs at once. A tenant runs at most 2 across replicas. |
| `GLOSSA_INTEGRATION_POLL_INTERVAL` | `1s` | Idle poll interval of an import/export worker. |
| `GLOSSA_INTEGRATION_JOB_TIMEOUT` / `_JOB_LEASE` | `30m` / `35m` | Budget for one attempt of a job; how long a claimed job is reserved before another worker resumes it from its last checkpoint. The lease must be longer. |
| `GLOSSA_INTEGRATION_MAX_UPLOAD_BYTES` | `67108864` | Largest import file (64 MiB; at most 2 GiB). The upload route streams it to object storage instead of taking `GLOSSA_HTTP_MAX_BODY_BYTES`. |
| `GLOSSA_INTEGRATION_UPLOAD_TIMEOUT` | `10m` | Read deadline of an upload and write deadline of a download, instead of the HTTP read/write timeouts. |
| `GLOSSA_INTEGRATION_RETENTION` | `168h` | How long uploaded and exported files are kept; the sweep deletes them afterwards (jobs and results stay). |

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
go test -tags=system -timeout=600s ./internal/systemtest/...  # M2 exit test (make system-m2); writes m2/REPORT.md
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
  refused. A context's in-process port that must write its own tables
  in its caller's commit joins the open transaction explicitly with
  `uow.InCurrentTenantTx(ctx, fn)` (`ErrNoTx` outside one) — how
  Localization's projection follows Catalog's bulk upsert.
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
like `translations.write`: translators and reviewers hold it within
their locales; developers, admins, owners and `write` tokens hold it
for every locale. The import/export permissions: every role and the
`read` scope hold `integration.read` (see jobs and results, create
exports and download them); `integration.import` (import translations
from XLIFF, JSON and PO) is **locale-scoped** like `translations.write`;
`integration.manage` (create and revise messages from files, TMX and TBX
imports, overwrite mode) belongs to owners, admins, developers and
`write` tokens.

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
overwriting a newer revision, obsolete keys reactivated. `GET
…/namespaces` lists a project's namespaces by name with their active
and obsolete message counts (what Studio's export offers to choose
from): one grouped read per page, served by the
`catalog_messages_namespaces` index (migration 0010).

Project settings: `default_syntax`, `review_required` and
`default_branch` — the repository's default branch (`main` unless set;
a Git connection will set it), validated as a Git branch name
(`invalid_branch`). Settings written without it keep the project's.
The Context context decides by it which uploads are of the default
branch. Settings live in `catalog_projects.settings` (jsonb); a project
saved before `default_branch` existed reads as `main`, and migration
0014 only bounds the stored value.

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
| `localization_messages` | tenant | Localization's own projection of Catalog messages, fed by `catalog.message.*` (highest `version` wins), updated in the same transaction by Catalog's bulk upsert and refreshed synchronously on every translation write. It is what "outdated" and the `missing_in`/`outdated_in` filters read. |
| `localization_translations` | tenant | Projection of the latest revision. |
| `localization_translation_revisions` | tenant | Append-only by grant. |

Localization reads Catalog only through Catalog's application service
(`adapters/catalog`, the `SourceCatalog` port) and answers Catalog's
coverage filter through `adapters/coverage`; neither context touches
the other's tables. **The projection is synchronous for bulk
upserts:** `UpsertMessages` (`message-upserts`, `glossa push`, imports)
hands the messages it changed to Catalog's `MessageProjection` port,
which `adapters/projection` implements with Localization's
`ProjectMessages`. That runs Localization's own projection update —
highest version wins, outdated translations announced — on Localization's
store *inside Catalog's transaction*, joined through the unit of work
(`db.UnitOfWork.InCurrentTenantTx`, an explicit join rather than a
nested unit of work), so listings and fills see a push when it returns
and a failure rolls both back. The outbox subscriber stays as the
idempotent catch-up (and still carries every other message write,
about one poll later): it finds the projection at the event's version
and changes nothing, so an outdated event is published once
(`TestBulkUpsertProjectsSynchronously`). Subscribers (names are stored with events; never
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
It ships with PostgreSQL's contrib modules — in the official images the
chart and the tests use (`postgres:16-alpine`) — and is a trusted
extension, so the database owner that runs `-migrate=only` creates it
without superuser rights. A cluster
built without contrib must provide it first.

**Translation memory.** Units are derived, not curated — or imported
from TMX (origin `import`, `ImportTMUnits`, run by Integration's jobs;
an exact duplicate in its scope is not added again):
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
as `{$total} zahlen` — in MF2 (`target`) and in the syntax asked for
(`target_text` in `target_syntax`; by default the query's own syntax,
so an MF1 query reads `{total, number} zahlen`). MF1 is written by
`mfcontent.RenderMF1`, the inverse of the MF1 conversion, and proven by
parsing it back to the same model; what MF1 can't express (markup,
MF2-only options) comes back as MF2 with `target_syntax_fallback`. `TestFuzzyMatchingQuality` pins quality on a
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
reports code point offsets. **Project checks**
(`CheckProjectTerminology`, `GET …/projects/{project}/terminology-findings`)
run the same QA server-side over a project's translations in up to 20
locales, each against its message's current source: a page scans
`page_size` translations of active messages in key order (default every
review state but `rejected`) and lists those with findings by message
key, with `checked` counts per locale. A page costs one read per
context — the termbase, Localization's bulk listing and Catalog's
`MessagesByIDs` for the page's sources (the `ProjectTranslations` port)
— never one per translation.

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

## Intelligence

AI translation that amplifies accumulated knowledge (RFC 0003 §3–§4,
intent §54). The library — the translation agent (`app.Translator`: gather
→ draft → validate → repair ≤ 2 → assess), the provider port and
adapters, confidence and review routing, prompts and evals — is wired
into the platform here.

| Table | Scope | Why |
|---|---|---|
| `intelligence_providers` | tenant | BYO providers: kind (`anthropic`, `openai_compatible`, `gemini`), base URL, model allow-list, enabled, the API key **sealed** (`kernel/sealing`, AES-256-GCM bound to tenant and row, key derived from `GLOSSA_AUTH_SECRET`). The key is write-only: the API says `api_key_set`, never the key. |
| `intelligence_settings` | tenant | Consent (`provider_consent`, off by default, who and when), `max_concurrent_jobs`, `monthly_budget_micro_usd` (0: no provider calls), price overrides. `glossa_system` may SELECT (the claim's cap). |
| `intelligence_project_settings` | tenant | Namespace tags (`sensitive`, `legal`, `marketing`), auto-translate locales (none by default), review routing. |
| `intelligence_routing_policies` | tenant | The tenant's (`project_id` NULL) and projects' routing; none stored means `DefaultRouting()`. |
| `intelligence_spend` | tenant | Every priced call, append-only: the budget is this UTC month's sum; nothing resets. Outlives projects. |
| `intelligence_fills` | tenant | Explicit and locale-added batch requests. |
| `intelligence_jobs` | tenant | The queue, with each job's audit ledger. `glossa_system` may SELECT and UPDATE (claims). |
| `intelligence_suggestions` | tenant | Results with provenance, score, explanation, action, risk tags, cost and the decision. |
| `intelligence_disclosures` | tenant | Which provider and model saw which message, and exactly what was sent. Append-only. |

**Settings documents** — the tenant's settings (and prices), the
tenant's and each project's routing policy, a project's settings —
exist with defaults before anyone saves them, at version 0: their
`ETag` is `"0"` (a project's routing policy answers `"0"` while it
inherits the tenant's), and `If-Match: "0"` writes one only while it is
still unsaved. A write that loses the race to create or change it
fails the precondition (`412`), never silently overwrites
(`TestWiringConcurrentCreatesOfSingletons`).

**Jobs.** A job is one message × locale, unique per `(message, locale,
source revision, knowledge fingerprint)` — the fingerprint digests the
prompt versions, the effective style guides' versions and the pair's
termbase (every concept in scope with a term in either locale, with its
version); translation memory is left out on purpose (it changes with
every approval). Triggers are idempotent outbox subscribers acting as
the background principal `intelligence.auto_translate` (catalog,
translations and knowledge read only): `catalog.message.created` and
`localization.translation.outdated` queue jobs for the locales with
auto-translate on, `localization.locale.added` fills a new locale with
auto-translate on (the fill's ID is the event's), and `POST
…/projects/{project}/ai-fills` queues explicit fills: the messages
whose translation is in the state `select` names (`missing`, the
default; `outdated`; `missing_or_outdated`), among listed keys or all of
them, so a client re-translates outdated text without listing keys
(failed, dead or cancelled jobs are queued again). A duplicate event finds the existing job.

**Previews.** `POST …/projects/{project}/ai-fill-previews`
(`PreviewFill`, `glossa translate --dry-run`) takes a fill request and
answers what the fill would do, writing nothing (no fill, job, TM hit
count, spend or event; `TestWiringFillPreviewWritesNothing` counts the
rows). Per locale it lists the keys, then decides each message the way
its job would: an existing job is reused (`existing`, one query per page
of 100), an exact TM match covers it (`tm_exact`, an exact-only lookup
that isn't counted as a hit), or it would call a provider (`provider`)
unless refused — `sensitive`, `provider_consent`, `no_route` (no route
to an enabled provider allowing the model), `budget_exceeded` (this
month's spend plus the calls before it leave no room for the call's
upper bound). `cost` prices the provider calls with the effective price
table: `estimated_micro_usd` (a draft and a self-assessment, prompts at
about three characters a token, drafts twice their source) and
`max_micro_usd` (what the budget guard reserves: every repair, the whole
`max_tokens`), with `unpriced` for models without a price.

Workers run in glossa-server (`GLOSSA_AI_*`). A claim is one statement in
the system scope `intelligence.jobs`, serialized by an advisory lock and
taken `FOR UPDATE SKIP LOCKED`: the next due job (queued, or running
with an expired lease) of a tenant under its `max_concurrent_jobs`, so
the cap holds across replicas. Each provider (tenant × configuration
version) is fortify-wrapped — timeout, retry with backoff, a circuit
breaker — and capped by an in-process bulkhead
(`GLOSSA_AI_PROVIDER_CONCURRENCY`). The job runs in its tenant's scope as
`intelligence.worker` (catalog, translations and knowledge read,
releases read, translations write and review) and ends:

| State | When |
|---|---|
| `succeeded` | A suggestion was stored (and routed). |
| `skipped` | `superseded` (the source moved on), `up_to_date` (translated meanwhile), `message_gone`, `locale_gone`. |
| `failed` | For good, with a reason: `provider_consent`, `sensitive`, `invalid_output`, `budget_exceeded`, `no_route`, `provider_error`, `invalid_source`. |
| `queued` again | A transient failure (`app.Transient`: rate limit, overload, outage): backoff 30 s doubling to 30 min, on the database's clock. |
| `dead` | Transient failures exhausted `max_attempts` (5), or its workers kept dying. |
| `cancelled` | By `POST …/ai-jobs/{job}/cancellation` or a fill's, while queued. |

Settlement is fenced by the claim token, so a worker whose lease ran out
changes nothing. Disclosures are stored before settlement, even for a
failed job: a provider that saw the text is recorded.

**Privacy (RFC 0003 §7).** Without `provider_consent` the agent never
calls a provider: an exact translation-memory match (100/101, clean
terminology, every plural category the target needs) is still reused,
anything else fails `provider_consent` with a reason that names the
setting, and a fill says so up front (`warnings: provider_consent_off`).
A namespace tagged `sensitive` is never queued by triggers, skipped by
fills (`skipped.sensitive`), refused by the worker if it was tagged after
queueing, and refused again by the agent's own tool guard. Base URLs are
https and never private or loopback addresses unless the deployment
allows them, checked on the resolved address when the client dials.

**Budgets.** `monthly_budget_micro_usd` caps provider spend per UTC
month; 0 (the default) allows none. Before each call the router asks the
guard whether this month's spend plus the call's upper-bound estimate
(input at the input price, the whole `max_tokens` at the output price)
fits, and refuses it otherwise (`budget_exceeded`, never retried, never
another route); after the call it books the actual cost. Calls already
in flight when the cap is reached finish, so a month can end at most
their cost over the cap. Prices are the deployment's defaults with the
tenant's overrides (`ai-prices`); a configured `anthropic`-kind provider
under another name gets the Anthropic prices under its name.

**Suggestions and review routing.** A suggestion stores the canonical
MF2, findings, provenance, score and explanation, and its action by the
project's review policy: `approve_recommended` and `review_required`
wait in the review queue (`GET …/ai-review-queue`, lowest score first,
then most risk tags: legal and marketing namespaces, forbidden terms,
max length, missing plural categories); a newer suggestion for the same
message and locale supersedes a pending one. Every suggestion the API
returns (list, get, queue, accept, reject) carries its message's
current `source` — key, namespace, state, authored text and syntax,
canonical MF2 and revision — read through Catalog's `MessagesByIDs` in
one query per project on the page (`SuggestionSources`), so reviewers
need no request per item. **`auto_approve`** is off by
default and is accepted only with `auto_approve_environments` that all
exist and ship `approved` (Release's eligibility policies —
`GetEnvironment` as `releases.read`); it is checked again for every
suggestion, and when an environment stopped shipping approved text the
suggestion is routed `approve_recommended` with an `action_note`. An
auto-approved suggestion is written as an approved revision by the
worker. Accepting (`…/acceptance`, optionally with an edit) writes the
revision through Localization as the caller (`intelligence.translate`
and `translations.write` for the locale; `approved` when the caller may
review, else the project's policy) with origin `ai` or
`translation_memory` and an `origin_detail` of provider, model, prompt
version, TM units, terms, style version, score, explanation, suggestion
and job. An edit records its structured diff: character edit distance
and ratio, terms added and removed, style fields whose use changed.
Rejecting writes nothing.

**Message context (RFC 0004 §2.2, §8).** The agent's `message_context`
tool gives the description, max length, where the message is used —
`file:line (component, route)`, at most 10, the default branch's first,
without repeats — and up to 5 neighbours with their translations (a
rejected one left out): first the messages shown together with it,
sharing a route or a capture in the default branch's current builds
(most shared first, active only), then those sharing its key prefix.
Usages and co-located messages come from the Context context through the
`UsageContext` port (`sources.Usages`, over `contextapp.Service`, as the
worker's principal with `catalog.read`); without it (`Deps.Usages` nil)
usages are empty and neighbours come from the key prefix alone. The
tool's output shape is unchanged, so prompts and eval cassettes are too.

**Plural categories (policy).** A draft lacking a CLDR plural category
the *target* locale needs (Polish `few`/`many`) goes back to the model
for repair like a structural error, even though `CheckCompat` calls it
a warning. If it is still missing after the two repairs, the suggestion
is kept, scored low (factor `missing_plural_categories` naming them) and
forced to `review_required` under every policy.

**Metrics.** Prometheus: `glossa_intelligence_jobs_total{trigger,state,code}`,
`glossa_intelligence_job_duration_seconds`,
`glossa_intelligence_cost_micro_usd_total{tenant,provider,model}`,
`glossa_intelligence_suggestions_total{locale,origin,action}`,
`glossa_intelligence_suggestion_confidence`,
`glossa_intelligence_suggestion_decisions_total{locale,status}`,
`glossa_intelligence_edit_ratio{locale}`, `glossa_intelligence_queue_jobs{state}`.
`GET …/projects/{project}/ai-metrics` returns the acceptance rate and
edit distance per locale (intent §70); `GET …/ai-eval-baseline` serves
the committed `evals/testdata/baseline.json`.

Subscribers: `intelligence.auto_translate` (the three triggers),
`intelligence.drop_project` on `catalog.project.deleted` (erases the
project's jobs, fills, suggestions, disclosures, settings and routing;
spend stays). Permissions: see *Identity* (`intelligence.read`,
`intelligence.manage`, `intelligence.translate`).

**Tests** replay provider cassettes, never real providers: the evals
(`evals/testdata/cassettes`) and the wiring's integration tests
(`app/testdata/wiring`, re-recorded from each test's scripted answers
with `go test -tags=integration ./internal/intelligence/app -run
TestWiring -record-wiring`).

## Integration

Interchange files in and out as asynchronous jobs (RFC 0003 §5–§6,
intent §48): the pure converters in `formats` (XLIFF 2.1, flat and
nested JSON, gettext PO read-only, TMX 1.4b, TBX-Basic), run by workers
in glossa-server against Catalog, Localization and Knowledge through
their application services.

| Table | Scope | Why |
|---|---|---|
| `integration_jobs` | tenant | Imports and exports: format, mode, options, the requester's access snapshot, state and progress, the file's object key, size and SHA-256, the fingerprint, summary counts, retention. `glossa_system` may SELECT and UPDATE (claims, the retention sweep). |
| `integration_job_items` | tenant | An import's result per item, in file order, with where the item is in the file (`line`, `col`, `ref`; migration 0011). |

**Files** live in object storage under
`integration/v1/<tenant>/jobs/<job>/…` (glossa-edge never serves that
prefix). An upload (`PUT …/import-jobs/{id}/file`, the requester only,
once) is streamed to storage while it is hashed and counted against
`GLOSSA_INTEGRATION_MAX_UPLOAD_BYTES` — that route takes no API body
limit and gets `GLOSSA_INTEGRATION_UPLOAD_TIMEOUT` as its deadline —,
and `objectstore.Streamer` writes it whole or not at all (a temp file
renamed into place; an S3 multipart upload aborted on failure). An
export is generated into a temporary file and streamed up; its download
(`GET …/export-jobs/{id}/file`) streams it back with its name and
SHA-256 (`ETag`). Retention deletes both after `expires_at`
(`GLOSSA_INTEGRATION_RETENTION`, 7 days): `integration.jobs`' sweep
lists expired files across tenants, deletes them and records
`files_deleted_at` (a download is then `410 file_expired`); imports
still waiting for their file after 24 hours fail `upload_expired`.

**Jobs.** `awaiting_upload` (imports) → `queued` → `running` →
`succeeded` | `failed` | `cancelled`. A claim is one statement in the
system scope `integration.jobs`, serialized by an advisory lock, `FOR
UPDATE SKIP LOCKED`, at most two running jobs per tenant across
replicas; new and queued jobs are due on the database's clock. The job
runs in its tenant as `integration.worker` (catalog, translations and
knowledge read and write, translations review) **within the access its
requester had when they asked**, snapshotted on the job
(`authz.ScopeOf`): the locales they may import (`integration.import` ∩
`translations.write`), the locales they may review, and whether they
may manage (`integration.manage` with `catalog.write`, or with
`knowledge.write` for TMX and TBX). An import applies its file in
batches of 500 entries, units or concepts; after each batch it stores
the batch's results and its progress under the claim token and checks
for a cancellation. A storage or database failure is retried (3
attempts, 30 s backoff doubling), resuming after the last checkpoint; a
malformed, unsupported or oversized file fails the job at once with the
problem as its last result (`line`, `column`, the item); a worker whose
lease ran out changes nothing. **Every result carries its position**:
the readers record where each entry, translation, unit and concept is
(`formats.Position`) — line and column of its start and `ref`, the item
in the format's own terms: an XLIFF 2 fragment identifier
(`#/f=checkout/u=pay`), a JSON pointer (`/checkout/pay`), a PO entry's
`msgctxt "…" msgid "…"`, `tu[n]` / `conceptEntry[n]` for TMX and TBX —
so a conflict deep in a file is found as easily as the problem that
fails it. Events: `integration.import.completed`
and `integration.export.completed` (job, project, kind, format, mode,
state, failure code, reused job, summary, requester) on every end;
subscriber `integration.drop_project` on `catalog.project.deleted`
deletes the project's jobs, results and files.

**Import rules.**

- *Locales.* A file's source locale must be the project's
  (`source_locale_mismatch`). Its translations' locale — XLIFF's
  `trgLang`, a PO file's `Language`, or `options.locale`, which names
  the locale of an XLIFF file without `trgLang` or imports one as
  another locale than it names (`de` into a project's `de-AT`) — must
  be one of the project's target locales: `options.locale` is checked
  when the job is created (`locale_not_found`), the file's own locale
  when it runs (the job fails `target_locale_mismatch`, naming the
  project's locales, instead of reporting every translation invalid).
- *Catalogs.* A message with source text in the file is created when
  missing (with `integration.manage`; otherwise its translations are
  `message_not_found`), `unchanged` when its source has the same MF2
  model, revised in `overwrite` mode and a `conflict` (`source_differs`)
  otherwise — a translator's XLIFF never rewrites the source it was
  exported with, and its translations of such a message are conflicts
  too (they were made for other text). Created messages take the file's
  namespace, description and max length; revisions change the source
  only. Translations go through Localization's bulk import with
  provenance `import` and `origin_detail` `{job, file, format,
  requested_by}`, written by `integration.worker`.
- *Merge never lowers an approval.* Under the translation's row lock
  (`localizationapp.ImportOptions.KeepApproved`): other text for an
  approved translation is a `conflict`
  (`approved_translation_conflict`), the same text in a lower state is
  `unchanged`. This protects every approval — a reviewer's, or an
  auto-approval policy a manager set — not only text a person typed.
- *Review states are capped* (`domain.RequestedState`): a file's
  approval (XLIFF `final`, PO without `fuzzy`) or rejection is kept only
  for someone who may review the locale, or — approvals — in a project
  that doesn't require review; otherwise the translation waits in
  `needs_review`. New text can't be imported as rejected
  (`write_cannot_reject`).
- *gettext keys* (`domain.POMessageKey`): `[<msgctxt slug>.]<msgid
  slug>_<hash>` — the text folded to lowercase ASCII words joined by `_`
  (accents dropped, other scripts left out, at most 40 characters at a
  word boundary) plus the first 8 hex digits of SHA-256 over msgctxt,
  U+0004 and msgid. The same entry always gets the same key, equal texts
  in different contexts different ones; `options.namespace` is every
  entry's namespace. Plurals become MF2 selects on `$count` with the
  target locale's CLDR categories.
- *TMX* adds units with origin `import` to a project or tenant-wide; a
  unit whose exact text is already active in the scope is `unchanged`,
  so TM imports only ever add (in either mode). *TBX* concepts are stored
  under their own ID when it is a UUID the tenant already has (a Glossa
  export coming back), else under one derived from the tenant, the
  scope and the file's ID (UUIDv5), so re-imports find them; identical
  content is `unchanged`, other content a `conflict`
  (`concept_differs`) unless `overwrite`. One definition and one note are
  kept per concept and term; the rest (other-language definitions, a
  term's context) joins the note. Case sensitivity, which TBX can't
  carry, is kept from the stored terms.
- *Dry runs* run every check a merge runs and store only the results:
  Localization's and Knowledge's writes (structural QA, the approval
  rule, concept and unit rules) run in transactions that are rolled
  back; Catalog's key, namespace and source rules are checked without
  writing, and the translations of messages that would be created are
  predicted `created`.
- *Idempotency.* `Idempotency-Key` on create, as everywhere. And an
  upload whose fingerprint (file SHA-256, project, format, mode,
  options) matches an earlier import that succeeded and hasn't expired
  succeeds at once with that job's summary and results
  (`reused_job_id`) instead of being applied twice; dry runs always run.

**Exports.** Catalogs read Catalog's `ReleaseSource` and Localization's
`ReleaseTranslations` (the chosen review states, default `approved`),
filtered by namespace: XLIFF one document per target locale (the source
alone without locales), JSON one catalog per locale — byte for byte what
`glossa pull` writes (sorted keys, two-space indent, no HTML escaping,
trailing newline); several locales are zipped as `<locale>.<ext>`. A
catalog the format can't express (MF2-only messages in an MF1 JSON
file, an XLIFF file without messages) fails `not_representable`. TMX
and TBX export a project's own units and concepts, or everything the
tenant holds without a project; TMX units carry the message key they
were approved for as `x-glossa-message-key`.

**The workspace's memory and termbase** have routes of their own, not
under a project: `POST|GET …/tm-import-jobs`, `…/tm-export-jobs`,
`…/termbase-import-jobs` and `…/termbase-export-jobs` create and list
tenant-wide TMX and TBX jobs (`CreateKnowledgeImport`,
`CreateKnowledgeExport`; the listing is `JobFilter{TenantWide, Kind}`).
Imports need `integration.manage` and `knowledge.write` held
tenant-wide, exports `integration.read` and `knowledge.read`. The jobs
are ordinary ones: uploaded to, followed, cancelled and downloaded
through `import-jobs` and `export-jobs`.

Permissions: see *Identity* (`integration.read`, `integration.import`,
`integration.manage`). **Tests**: `internal/integration/app` runs every
format end to end on Postgres (app role, no BYPASSRLS) and MinIO —
including the XLIFF locale option, per-item positions and the
workspace's routes; `cmd/glossa-server` uploads, imports, exports and
downloads through the generated server.

## Context

Where every message appears (RFC 0004 §2–§3, intent §16–§18, §68). The
Go packages live in `internal/context/`; their names (`domain`, `app`,
`postgres`, `catalog`) never clash with the standard library's
`context`, and callers alias them `contextapp`, `contextpg` and so on.
Its HTTP edge is `adapters/httpapi` (the Context API below), its
Prometheus metrics `adapters/metrics`; Intelligence reads it through
its own port (`sources.Usages`).

- A **build** is one upload for one application at one commit: branch,
  whether that branch is the repository's default, source (`plugin`,
  `extract`, `runtime` or `capture`), tool and the SHA-256 digest of
  the uploaded document. An upload is idempotent by (application,
  commit, source, digest): a repeat is a replay.
- A **usage** is key × file, line, column, component, route and kind
  (the call shape) in a build. Keys are resolved to message IDs **at
  ingest** through Catalog's port, so a rename never touches a usage;
  a key the catalog doesn't know is stored with a null ID (an *unknown
  key*, counted in the event).
- A **capture** is one (route, viewport, locale) screenshot of a build:
  the image's digest and size (the image itself is content-addressed in
  object storage, RFC 0004 §3.3). Its **regions** are the boxes
  (`element`, `text` or `attribute`, `visible` or not) of the messages
  rendered on it, resolved like usages.
- Limits (§10): a document of at most 20 MB and 100 000 usages; 500
  captures per build (held under an advisory lock on the build); 10 000
  regions per capture; images of at most 40 megapixels.

`IngestUsages` takes a `glossa.usages/v1` document
(`domain.UsagesDocument`; field names exactly as RFC 0004 §2.2) and the
source. `domain.ParseUpload` validates it by the schema's rules
(`runtimes/testdata/schemas/usages.v1.schema.json`: full lowercase
commit IDs, slugs, keys, relative POSIX paths, `\S+` components, route
patterns, the five kinds, a semver tool version); `schema_test.go`
holds it to the schema itself (santhosh-tekuri) over the example,
every usage fixture and the checker's variants. The deliberate
deviations are listed there: members the schema doesn't define are
ignored within v1, so a newer collector's optional field doesn't break
an older server, and branch names Git refuses (`@`, `-x`, a `.lock`
component) are refused. Whether a build is of the default branch is
Catalog's `settings.default_branch` (`main` unless set), read through
the port at ingest — never the uploader's say; a later change of the
setting leaves existing builds as they were. Uploads go through a
per-tenant `Limiter` (the kernel's token bucket: 10 a minute, bursts of
60). `IngestCapture` adds a capture to a build of the project. Both need
`catalog.write` (developers, `write` tokens: CI) and publish
`context.build.ingested` (`build_id`, `project_id`, `application_id`,
`commit`, `branch`, `on_default_branch`, `source`, `usages`,
`unknown_keys`, `by`) and `context.capture.ingested`.

**Current usages.** Collectors are independent, so "the latest build of
each application" is taken per application **and source**: the Go
extractor's build never hides the bundler plugin's usages, and a
capture build with no usages never makes everything unused. The
default view is the latest default-branch build per (application,
source); a branch view uses the branch's latest build per (application,
source) and falls back to the default branch's where the branch didn't
rebuild (`domain.CurrentBuilds`, a pure function over the project's
build summaries). `MessageUsages`/`KeyUsages`/`UsagesOfKey` list a
message's current usages, default branch first; `ListUsages` pages
through the current usages on a route, in a component or in a file (the
messages a screen shows, §8); `CoLocated` returns the messages sharing a
route or a capture with one, most shared first (the agent's
neighbours); `ListBuilds` pages through a project's builds, newest
first, with their unknown keys; `UnusedMessages` lists the active
messages without a current usage — reported, never obsoleted. Reads
need `catalog.read`.

**The Context API** (`api/openapi.yaml`, tag `context`):

| Operation | What |
|---|---|
| `POST …/projects/{project}/context-builds?source=` | Upload a `glossa.usages/v1` document (≤ 20 MB; the route lifts the body limit): `201` with the build, or `200` + `Idempotent-Replayed` for the same document again. The generated server decodes the body into the contract's shape (dropping undefined members); the digest is the SHA-256 of its RFC 8785 canonical form. `invalid_usages`, `too_many_usages`, `invalid_source`, `unknown_application` (400), `payload_too_large` (413), `rate_limited` (429). `glossa context push` and `glossa extract --upload` call it. |
| `GET …/context-builds[?application=]` | The builds, newest first, with `usages` and `unknown_keys`. |
| `GET …/messages/{message}/usages[?branch=&limit=]` | A message's current usages (`truncated` past `limit`, 1–1000). |
| `GET …/usages[?route=&component=&file=&branch=]` | The current usages matching, by build and position. |
| `GET …/unused-messages[?branch=]` | The active messages without a usage, by key, with `current_builds`, `active_messages` and `unused_messages`. |

**Metrics** (RFC 0004 §11): `glossa_context_builds_total{source,
outcome}` (stored, replayed), `glossa_context_usages_ingested_total`
and `glossa_context_unknown_keys_total{source}`, and
`glossa_context_coverage_ratio{tenant, project}`: the share of active
messages with a current default-branch usage, measured by the
subscriber `context.measure_coverage` (background principal, catalog
read) after every default-branch build and by every default-view
`UnusedMessages` read. Each instance reports what it measured last;
take the max across instances.

**Retention** (`domain.RetentionPolicy`, §2.3): per (application,
branch, source) the latest 5 builds are kept, plus every current one; a
closed branch's builds go 14 days after it closed (default-branch builds
never do). `PurgeProject` applies it and returns the deleted builds and
the images no remaining capture references, for the caller to delete
from object storage. `Purge` visits every project holding builds through
the system scope `context.retention` (migration 0012 opens only
`context_builds.tenant_id` and `project_id` to `glossa_system`) and
purges each in its tenant as the background principal `context.purge`.
Scheduling it daily, and deleting images, come with the purge jobs
(RFC 0004 §13, wave 7). Closed branches come from Catalog's branch
overlay (§4.1); until then the port reports none.

| Table | Scope | Why |
|---|---|---|
| `context_builds` | tenant | Keyed by Catalog's project and application IDs (no cross-context foreign keys). SELECT, INSERT, DELETE: immutable. System scope reads `tenant_id` and `project_id` only. |
| `context_usages` | tenant | A build's usages by position, with the message ID resolved at ingest; cascade with their build. Indexed by message and (migration 0014) by route, for the co-located messages. |
| `context_captures` | tenant | One per (build, route, viewport, locale); cascade with their build. |
| `context_regions` | tenant | A capture's regions by position; cascade with their capture. |

Subscribers: `context.drop_project` on `catalog.project.deleted` and
`context.drop_application` on `catalog.application.deleted` erase what
they held; `context.measure_coverage` on `context.build.ingested`
measures coverage. **Tests**: `internal/context/domain` (documents and
the schema, limits, current views, retention) and `internal/context/app`
on Postgres (app role): ingest, replay, batches, unknown keys, the
project's default branch, renames, branch views, builds and usages
pages, co-located messages, unused messages, the upload limit, metrics
and coverage, captures and their limit, retention with orphaned images,
the cross-tenant sweep and tenant isolation; `adapters/httpapi` (routes,
problem codes); `cmd/glossa-server` runs the API over HTTP with a write
token (uploads, replays, refusals, reads, the coverage metric) and
checks the composition.

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
`adapters/ratelimit` implements with the kernel's fortify token bucket
(`kernel/ratelimit`, which Context's upload limit uses too) per
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

**Branch environments** (RFC 0004 §4.2) preview one open branch each:
`kind: branch`, named `pr-<number>` or `br-<first 8 hex of
sha256(branch)>` (names no standard environment may take), with the
fixed preview policy (draft, needs_review and approved, outdated
included; `fixed_policy` otherwise). Their build is the main catalog
plus that branch's overlay only (`domain.BuildBranch`): its proposed
messages with their translations, and its source proposals in the
source locale. Every other build keeps excluding both. A branch release
records its branch and can't be promoted
(`branch_release_not_promotable`); a project has at most 50 branch
environments (`too_many_branches`). The lifecycle hooks the Branches API
and the GitHub webhooks will call: `OpenBranchEnvironment`
(create-on-open, idempotent per branch), `RequestBranchPublish`
(publish-on-push, debounced: a request moves the pending publish to 30 s
after it, at most 5 minutes after the first; the `Publisher` runs due
requests across tenants as `release.publisher`, with the request's ID as
the publish's idempotency key) and `DestroyBranchEnvironment` (deletes
the environment and its manifest, so the edge answers 404; releases and
deployments stay).

**Delivery keys are scoped** (RFC 0004 §4.3, SPEC §2): an `environments`
allowlist and a `branches` flag, written into the key index object and
enforced by the edge, which answers 404 outside the scope exactly as for
an unknown key. New keys read `production` only; a preview key
(`branches: true`) belongs in preview deployments. Migration 0015 gave
existing keys the four default environments, and glossa-server's key
index task (`RewriteKeyIndexes`, run at startup until it succeeds)
rewrites their index objects; until then the edge reads an object
without `environments` as those four. The API doesn't expose scopes yet,
so keys it creates keep the four default environments until it does.

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
v1/keys/<sha256(key)>.json                               delivery key → project and scope, while the key is active
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
| `release_environments` | tenant | Policy, kind (and branch) and current release per environment. |
| `release_deployments` | tenant | Every pointer move, append-only by grant: what rollback walks. |
| `release_delivery_keys` | tenant | Publishable keys, in the clear (they ship in bundles), with their scope; revoked ones stay listed. `glossa_system` reads IDs and `index_version` (system scope `release.key_index`). |
| `release_publish_requests` | tenant | Pending debounced publishes of branch environments. `glossa_system` reads IDs and `not_before` (system scope `release.publisher`). |

Events: `release.published`, `release.promoted`, `release.rolled_back`
(the Release aggregate shares the context's name, so they are
`release.<verb>`, as SPEC §3 names them),
`release.environment.{created,policy_changed,destroyed,publish_requested}`,
`release.delivery_key.{created,revoked}` (never carrying the key).
Subscribers: `release.sync_manifest` and `release.sync_delivery_key`
(storage writes; `sync_manifest` also removes a destroyed environment's
manifest), `release.retire_project` on `catalog.project.deleted`
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
revoked keys 404, and so does a manifest outside the key's scope (its
`environments`, plus branch environments with `branches`; checked before
storage is read, conformance fixture `runtimes/testdata/edge/`), storage
failures 503 `no-store`, objects failing their
integrity check 502. An in-process LRU caches keys, manifests and
artifacts with singleflight on misses, and serves stale entries through
a storage outage. `/readyz` ignores storage on purpose, so an outage
can't pull every edge out of the load balancer.
