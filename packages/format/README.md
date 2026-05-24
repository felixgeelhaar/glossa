# `@felixgeelhaar/glossa-format` — ICU subset (~200 LOC)

Stub. Implementation lands first in this monorepo — it has no dependencies on the rest.

## Scope

| ICU feature | Status | Backed by |
|---|---|---|
| Variable interpolation `{name}` | planned | own parser |
| Plural `{count, plural, one {…} other {…}}` | planned | `Intl.PluralRules` |
| Select `{gender, select, female {…} other {…}}` | planned | own parser |
| Nested combinations | planned | recursive eval |
| Apostrophe escaping | planned | own lexer |

## Defer to browser built-ins

- Numbers → `Intl.NumberFormat`
- Dates → `Intl.DateTimeFormat`
- Relative time → `Intl.RelativeTimeFormat`
- Lists → `Intl.ListFormat`

## Out of scope (MVP)

- Custom formatter extension API
- Bi-di / RTL handling (defer until first RTL tenant)
- Complex grammatical gender beyond binary + neutral
