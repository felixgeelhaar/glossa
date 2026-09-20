# Agent guide

Glossa is becoming localization infrastructure. Before you design or change anything:

1. **[`docs/product-intent.md`](./docs/product-intent.md)** is the north star. When an implementation detail is ambiguous, pick the option that moves toward it. Check your change against §63 (the coding-agent questions) and follow §74 (the decision hierarchy) when goals conflict.
2. **[`docs/rfcs/0002-platform-architecture.md`](./docs/rfcs/0002-platform-architecture.md)** is the architecture of the rewrite: bounded contexts, MessageFormat 2 model, control/delivery split, runtimes, milestones and dogfood waves. Don't reopen a decision there without writing a new RFC. RFC 0001 is kept for its reasoning, but its "evolve in place" plan is superseded.
3. **[`docs/design.md`](./docs/design.md)** is historical. It describes how v0.1–v0.3 was built, and RFC 0001 §6 lists what it supersedes.

Rules that come up most often:

- Messages, not files. JSON, XLIFF and PO are import/export formats, never the model.
- Standards first: BCP 47 tags, CLDR data through `Intl.*` and `golang.org/x/text`, the Unicode MessageFormat 2 data model (ICU MF1 as an authoring syntax).
- The rewrite lives in `platform/`, `messageformat/`, `runtimes/`, `studio/` and `site/`. `apps/` and `packages/` are v0.3: security and data-loss fixes only until it's retired.
- Follow the Klarlabs product standard: Go DDD/hexagonal, Postgres with forced RLS, `auth-go`, Vue + `@klarlabs-studio/ui`, first-party libraries.
- Important state is versioned. Translations carry provenance, and releases are immutable.
- Anything that matters is reachable through the API, not only through the admin UI.

Layout and commands: see the README's *Project layout* and *Development* sections. Work is planned in Roady (`.roady/`).
