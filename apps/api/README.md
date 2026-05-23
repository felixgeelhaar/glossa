# `apps/api` — Glossa REST + SSE service

Stub. Implementation lands after the Glossa kickoff (gated on IRI v0.2.0 — see top-level `README.md` § Decisions locked).

## Layout (planned)

```
cmd/api/             — main.go entry point
internal/
  domain/            — pure domain types (Project, Locale, Key, Translation)
  app/               — use cases (Translate, ApproveTranslation, RotateAPIKey)
  interfaces/
    httpchi/         — REST + SSE handlers (chi router)
  infra/
    sqlcadapter/     — sqlc-generated repo adapters
  db/                — sqlc-generated model + querier
db/
  migrations/        — golang-migrate up/down pairs
  queries/           — sqlc .sql sources
```

Mirrors IRI's apps/api layout intentionally — hex-arch, sqlc, golang-migrate.

## Status

- [ ] `go mod init github.com/felixgeelhaar/glossa/apps/api`
- [ ] Initial migrations (tenants, projects, locales, keys, translations, audit_log, users)
- [ ] Domain layer
- [ ] REST endpoints per `docs/design.md` § 5.1
- [ ] SSE channel per § 5.2
- [ ] RLS policies per § 6
