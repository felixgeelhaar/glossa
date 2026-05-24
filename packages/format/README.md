# `@felixgeelhaar/glossa-format`

ICU MessageFormat subset — variable interpolation, plurals via `Intl.PluralRules`, select, nesting, apostrophe escaping. Zero runtime dependencies. ~400 LOC, ~9 KB unpacked.

```bash
pnpm add @felixgeelhaar/glossa-format
```

## Usage

```ts
import { format, parse } from "@felixgeelhaar/glossa-format";

format("Hello, {name}!", "en", { name: "Sophia" });
// → "Hello, Sophia!"

format(
  "{count, plural, one {# session} other {# sessions}}",
  "en",
  { count: 3 },
);
// → "3 sessions"

format(
  "{gender, select, female {She did it} male {He did it} other {They did it}}",
  "en",
  { gender: "female" },
);
// → "She did it"
```

For hot paths, parse once and reuse the AST:

```ts
const ast = parse("{count, plural, one {# tip} other {# tips}}");
format(ast, "de", { count: 1 }); // → "1 Tipp"
```

## Supported ICU features

| Feature | Example |
|---|---|
| Variable interpolation | `{name}` |
| Plural categories | `{n, plural, zero{…} one{…} two{…} few{…} many{…} other{…}}` |
| Numeric plural override | `{n, plural, =0{none} one{…} other{…}}` |
| Hash placeholder inside plural | `{count, plural, one {# item} other {# items}}` |
| Select | `{gender, select, female{…} male{…} other{…}}` |
| Nested patterns | plurals inside select inside variable substitution |
| Apostrophe escaping | `it''s` → `it's`, `'{literal}'` → `{literal}` |

Plural keyword resolution defers to `Intl.PluralRules` so the package works across the ~100 locales the browser ships without bundling CLDR. Falls back to `"other"` on environments without `Intl.PluralRules`.

## Out of scope

- Numbers / dates / relative time — use `Intl.NumberFormat`, `Intl.DateTimeFormat`, `Intl.RelativeTimeFormat` directly.
- Custom formatter extension API.
- Bidi / RTL re-ordering.

## License

MIT
