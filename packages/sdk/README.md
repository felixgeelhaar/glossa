# `@glossa/sdk` — HTTP fetch + cache + SSE

Stub. Implementation lands after `@glossa/format` is testable.

## Responsibilities

- Initial locale-bundle fetch on `<glossa-provider>` boot
- In-memory cache keyed by `(project, locale)` with etag/version handling
- SSE subscription → emit events into the provider's reactive store
- Build-time pull (consumed by `@glossa/cli`)

Does NOT own: component lifecycle, rendering, formatting.
