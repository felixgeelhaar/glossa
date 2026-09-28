# `glossa` — the Dart runtime

The Dart implementation of [`runtimes/SPEC.md`](../SPEC.md), the contract every
Glossa runtime follows. It is **pure Dart**: no Flutter import, no `dart:io` in
`lib/`, no FFI and no reflection, so it compiles for the web and for AOT and can
be tested with `dart test`. Designed in [RFC 0005 §6](../../docs/rfcs/0005-quality.md).

## What works today (wave 1)

| SPEC | Here |
|---|---|
| §1.1–1.2 manifest and artifact | `Manifest.decode`, `Artifact.decode`. Unknown fields ignored, a different `schema` major rejected, one unreadable message reported and skipped rather than failing the release |
| §4.1 negotiation | `canonicalizeLocale` (RFC 5646 §4.5), `lookupLocale` (RFC 4647 §3.4), `resolveLocales`, `acceptLanguage` |
| §4.2 fallback chain | `fallbackChain`: explicit edges depth-first, truncations, `"*"`, the source locale, cycles stopped at the first repeat |
| §4.3 message resolution | `Localizer.t` formats with the locale the message was **found** in |
| §5 formatting | `formatMessage` / `formatToParts`: a MessageFormat 2 interpreter over the precompiled data model. No parser. Formatting never throws |
| §6 observability | `Localizer.explain` returns the SPEC document verbatim; `Catalog.errors` is the error channel with the six types and 60-second repeat suppression. **No telemetry** |

```dart
import 'package:glossa/glossa.dart';

final catalog = Catalog.fromRelease(
  manifest: Manifest.decode(manifestBytes),
  artifacts: {sha256: artifactBytes},
  source: Source.bundled,
);
catalog.errors.listen((e) => log.warning('$e'));

final t = catalog.forLocales(['de-AT', 'en']);
t.t('cart.checkout');                     // "Zur Kassa"
t.t('cart.items', values: {'n': 3});      // "3 Artikel"
t.direction;                              // Direction.ltr
t.explain('cart.checkout').chain;         // [de-AT, de, en]
```

## What is coming

- **Wave 2 — loading and integrity.** The SPEC §3 load order (memory →
  persisted → network → bundled → inline), SHA-256 integrity, Ed25519 over the
  RFC 8785 (JCS) canonicalization of the manifest, and the
  [`testdata/loading`](../testdata/loading) fixtures. Nothing here fetches or
  verifies anything yet: `Catalog.fromRelease` trusts the bytes it is handed,
  exactly as SPEC §3 says bundled artifacts are trusted.
- **Wave 3 — Flutter.** `package:glossa/flutter.dart`, a `GlossaText` that turns
  formatted parts into a `Text.rich`, and the
  [`markup.json`](../testdata/markup.json) safe-tag contract. Markup already
  reaches `Localizer.parts` as `MarkupPart`; nothing renders it yet.
- `glossa generate`'s typed accessors for Dart.

## Running the fixtures

The tests are a **driver over the shared fixtures**, not a copy of them
(RFC 0005 decision 9). They read `runtimes/testdata` and
`messageformat/testdata` from the repository and fail loudly if either is
missing, so they only run inside a checkout:

```sh
cd runtimes/dart
dart pub get
dart test                       # everything
dart test test/scenarios_test.dart
dart analyze --fatal-infos      # the CI bar
```

CI runs the same three commands on a pinned SDK in the `runtimes-dart` job.

| Suite | Source | Status |
|---|---|---|
| `test/scenarios_test.dart` | `runtimes/testdata/scenarios/*.json` | every case, **no skips** |
| `test/runtime_format_test.dart` | `messageformat/testdata/glossa/runtime-format.json` | 89 of 130; 41 skipped, each with its reason |
| `test/locale_test.dart` | mirrors `runtimes/go/locale_test.go` and the JS locale suite | — |

A bug found here becomes a fixture first (SPEC §7).

## What the host's CLDR decides

Numbers, dates and plural categories come from
[`package:intl`](https://pub.dev/packages/intl), which carries its **own** CLDR
snapshot — currently CLDR 48, the version `runtime-format.json` was generated on
(`generatedWith.cldr`). Glossa ships no locale data of its own, so the host's
`intl` version decides the output, and an app that pins a different `intl` can
see different strings.

`messageformat/testdata/glossa/README.md` sets the rule, and this package
follows it rather than inventing one:

> An implementation that can't reproduce a case keeps a skip list with the
> reason and the upstream gap, never a workaround.

The fixture's cases are chosen so their output is identical in CLDR 47 and 48,
so a mismatch is never version drift — it is a gap, and it gets fixed upstream.
`_skips` in `test/runtime_format_test.dart` is the list, kept honest in both
directions: an entry whose case has disappeared fails the suite, and so does one
whose case would now pass. The gaps today, all in `package:intl`:

| Gap | Cases | Effect |
|---|---|---|
| No ordinal plural rules | 15 | `:number select=ordinal` can't select; `bad-selector` |
| No measurement-unit data | 7 | `:unit` is **not registered**, so it is `unknown-function` and renders its MF2 fallback — visible, rather than silently wrong |
| No time-zone database | 9 | a named `timeZone` other than `UTC` reports `unsupported-operation` and formats in the value's own zone |
| One global currency-symbol table | 4 | a currency foreign to the locale keeps its symbol instead of falling back to the ISO code (es/fr `JPY`), and `ja` misses U+FFE5 |
| No currency display names | 1 | `currencyDisplay=name` reports `unsupported-operation` |
| CLDR `minimumGroupingDigits` ignored for `es` | 3 | `1.234` where CLDR says `1234`. The same gap `runtimes/go` records for go-intl |
| Plural operand `i` is `round()`, not the integer part | 1 | `fr` 1.5 gets `i=2`, so *other* instead of *one* |
| Stale German `dateTimeFormat` | 1 | `'{1}, {0}'` at every length, where CLDR 48 has `{1} 'um' {0}` for full and long |

Locale **identity** does not depend on any of this. BCP 47 canonicalization is
RFC 5646 §4.5 against the IANA Language Subtag Registry, whose tables
`tool/gen_subtags.dart` generates into `lib/src/subtags.g.dart` — Dart has
neither ICU's `getCanonicalLocales` nor `golang.org/x/text`, so the data is
generated rather than guessed, and the generated file records the registry's
`File-Date`. Regenerate with the command at the top of the generator.
