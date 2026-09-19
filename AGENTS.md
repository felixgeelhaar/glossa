# Agent guide

Glossa is becoming localization infrastructure. Before you design or change anything:

1. **[`docs/product-intent.md`](./docs/product-intent.md)** is the north star. When an implementation detail is ambiguous, pick the option that moves toward it. Check your change against §63 (the coding-agent questions) and follow §74 (the decision hierarchy) when goals conflict.
2. **[`docs/rfcs/0001-platform-foundation.md`](./docs/rfcs/0001-platform-foundation.md)** fixes the Phase 1 decisions (D1–D10) and the slice order. Don't reopen a decision there without writing a new RFC.
3. **[`docs/design.md`](./docs/design.md)** is historical. It describes how v0.1–v0.3 was built, and RFC 0001 §6 lists what it supersedes.

Rules that come up most often:

- Messages, not files. JSON, XLIFF and PO are import/export formats, never the model.
- Standards first: BCP 47 tags, CLDR data through `Intl.*` and `golang.org/x/text`, ICU MessageFormat.
- Never break a running consumer. v1 endpoints and `<glossa-text>` stay compatible (RFC 0001 §5).
- Important state is versioned. Translations carry provenance, and releases are immutable.
- Anything that matters is reachable through the API, not only through the admin UI.

Layout and commands: see the README's *Project layout* and *Development* sections. Work is planned in Roady (`.roady/`).
