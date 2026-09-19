# @glossa/runtime

The JavaScript runtime's formatter: a minimal **MessageFormat 2 interpreter
over precompiled messages**. Release artifacts carry the canonical data model
(`message.schema.json`), not source text, so no parser ships to the browser.
Formatting uses `Intl.*` only and has no dependencies.

The loader, cache, locale resolver and `explain` of the runtime contract
(RFC 0002 §8) come in M1, on top of this.

```ts
import { format, formatToParts } from "@glossa/runtime";

format(message, "de", { count: 3 }); // "3 neue Nachrichten"
formatToParts(message, "de", { count: 3 }); // text, number, markup, bidiIsolation, fallback parts
format(message, "de", {}, { onError: (e) => log(e.type, e.source) }); // "Hallo {$name}!"
```

`format(message, locale, values?, opts?) → string` and
`formatToParts(…) → Part[]`. `locale` is a BCP 47 tag or a list of them.
Options:

| Option | Default | |
|---|---|---|
| `onError(error)` | none | Called with `{ type, source }` per error (`unresolved-variable`, `bad-operand`, `bad-option`, `bad-selector`, `unknown-function`, `not-formattable`, `unsupported-operation`, `bad-message`). |
| `bidiIsolation` | `"default"` | Wrap placeholders in Unicode isolates (U+2066–2069) as the spec requires; `"none"` turns that off. |
| `dir` | from the locale | The message's base direction. |
| `functions` | none | Custom functions (`MessageFunction`), merged over the built-ins. |

## What it implements

- Patterns, `.input` and `.local` declarations (resolved lazily, once),
  `.match` with the spec's pattern-selection algorithm (exact keys before
  plural categories, `*` fallback, any number of selectors).
- Functions `:string :number :integer :percent :currency :offset :date :time
  :datetime :unit` with the spec's (LDML 48) options, on `Intl.NumberFormat`,
  `Intl.PluralRules` and `Intl.DateTimeFormat`. Intl formatters are cached per
  locale and options.
- Markup as structured parts (`{ type: "markup", kind, name, options?, id? }`);
  it formats to nothing in `format()`. Attributes are ignored, as the spec says.
- `u:dir` and `u:id` on expressions; bidi isolation per the spec, with each
  value's direction taken from CLDR (`Intl.Locale` text info).
- **It never throws.** A failing placeholder renders as its fallback
  (`{$name}`, `{|literal|}`, `{:fn}`) and reports through `onError`. A message
  it can't interpret at all (malformed data) renders as `{�}` and reports
  `bad-message`; the M1 loader then falls back along the locale graph.

## Conformance

`pnpm test` runs:

- the **vendored Unicode MessageFormat suite** (`messageformat/testdata/unicode`):
  each `src` is parsed with the reference parser *in the test only*, then
  interpreted here and checked against `exp`, `expParts` and `expErrors`.
  Every runtime case passes; the skip list in `src/conformance.test.ts` is
  empty. Syntax and data model errors are compile-time, so for those the test
  only checks that the reference parser rejects them.
- Glossa's **`messageformat/testdata/glossa/runtime-format.json`**: German and
  English UI strings (plurals with exact keys, ordinals, EUR, dates, nested
  selectors, MF1 offsets, markup) plus Arabic and Hebrew bidi cases, with
  expected output from the reference formatter.

The tests need `@glossa/messageformat` built first
(`pnpm -r --filter "./messageformat/js" --filter "./runtimes/js/*" build`).

## Size

Budget: 4 kB brotli for the core (RFC 0002 §8). This package, minified and
brotli-compressed with everything above, measures **3.13 kB** (`pnpm size`).

Spec-complete bidi and `:unit` are in: `:unit` shares the number code path and
costs a few bytes, and bidi isolation is a few lines once each value knows its
direction. The biggest single piece is the date/time option mapping. That
leaves about 0.9 kB for the M1 loader, cache and resolver. If they don't fit,
the next step is to trim here, not to drop spec behaviour: the obvious
candidates are the `Intl.Locale` script fallback in `dirOf` (only needed where
`textInfo` is missing) and the date/time option validation.

## Deliberate differences from the reference implementation

- Dotted variable names aren't looked up as paths into nested values
  (`{$user.name}` needs a `user.name` key). The reference does this as an
  extension; the spec doesn't.
- `timeZone=input` keeps the operand's zone, and converting between zones
  isn't reported as an error.
- `:number select=plural` means cardinal plural rules (the reference passes
  `type: "plural"` through to `Intl.PluralRules`, which rejects it).
- Custom functions return `toParts()` only; `format()` joins the parts.
