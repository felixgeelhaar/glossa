# platform

Go module for Glossa's control plane (RFC 0002). Today it holds the
`glossa-server` kernel: configuration, observability, the HTTP edge,
Postgres with forced row-level security, tenancy and the transactional
outbox. Bounded contexts build on it under `internal/<context>/`.

```text
cmd/glossa-server/          composition root only
db/migrations/              golang-migrate SQL, embedded into the binary
db/queries/<context>/       sqlc input, one directory per context
sqlc.yaml                   one sqlc entry per context
internal/kernel/
  config/                   env config, validated at startup
  observability/            bolt logger, OTel tracer, Prometheus, request middleware
  httpserver/               net/http server, /livez /readyz /metrics
  db/                       pgx pool, migrator, unit of work, dbtest harness
  tenancy/                  tenant ID/kind, context, Resolver middleware; tenantpg adapter
  outbox/                   Publish, Registry, Dispatcher, Postgres store
  problem/                  RFC 9457 error bodies
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
go run ./cmd/glossa-server -migrate=only                 # creates roles, tables, policies
psql "$MIGRATION_DATABASE_URL" -c "ALTER ROLE glossa_app LOGIN PASSWORD 'app'"
go run ./cmd/glossa-server
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

### Tests

```sh
go test -race ./...                            # unit
go test -tags=integration -timeout=300s ./...  # Docker: Postgres 16 via testcontainers
go generate ./db/...                           # regenerate sqlc code (sqlc pinned in db/generate.go)
```

The integration harness (`internal/kernel/db/dbtest`) provisions the
database the way production does: a `CREATEROLE` owner runs the
migrations, and the tests connect as `glossa_app`.

## Contract for context authors

### Tenancy and row-level security

- Every tenant-owned table has a `tenant_id uuid NOT NULL` column plus
  this block in its migration. The RLS guard test (`TestRLSGuard`) reads
  the catalog and fails CI for any table that doesn't. A table without
  tenant data goes in the guard's `globalTables` with a reason.

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
  whose `Resolver` (Identity) derives it from the session or token,
  never from a header. Pass the `*db.TenantTx` to your sqlc
  `New(tx)`. The transaction commits when `fn` returns nil. Nested units
  of work are refused.
- `uow.InSystemTx(ctx, db.NewSystemScope("ctx.job"), fn)` is for
  tenantless background work only. It refuses a context that carries a
  tenant, and it runs as `glossa_system`, which sees nothing unless a
  migration grants it a table and adds a `TO glossa_system` policy.
  That policy must then be listed in the guard's `systemPolicies`.

### Outbox

```go
err := uow.InTenantTx(ctx, func(ctx context.Context, tx *db.TenantTx) error {
    // … change state with sqlc on tx …
    _, err := outbox.Publish(ctx, tx, outbox.Event{
        Type: "catalog.source_revised", AggregateType: "message", AggregateID: id,
        Payload: SourceRevised{Revision: rev},
    })
    return err
})

events.Subscribe("catalog.source_revised", "localization.mark_outdated",
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
