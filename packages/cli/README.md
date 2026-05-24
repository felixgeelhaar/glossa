# `@felixgeelhaar/glossa-cli` — build-pipeline integration

Stub. Implementation lands after the API ships `keys:scan` and bundle endpoints.

## Commands (planned)

```
glossa init                 — scaffold a glossa.config.{ts,json}
glossa scan                 — walk source, extract keys, POST to /keys:scan
glossa pull                 — fetch all locale bundles to disk for build-time baking
glossa push                 — push a translated bundle back (translator workflow)
```

The CLI is build-time only — runtime updates come via SSE in `<glossa-provider>`.
