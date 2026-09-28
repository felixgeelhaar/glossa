# `glossa` — the Dart runtime

The Dart implementation of [`runtimes/SPEC.md`](../SPEC.md), the contract every
Glossa runtime follows. It is **pure Dart**: no Flutter import, no FFI and no
reflection, and no `dart:io` in `package:glossa/glossa.dart`, so the core
compiles for the web and for AOT and can be tested with `dart test`. The pieces
that need a file system or a socket live in the separate entry point
`package:glossa/io.dart`. Designed in
[RFC 0005 §6](../../docs/rfcs/0005-quality.md).

## What works today (waves 1 and 2)

| SPEC | Here |
|---|---|
| §1.1–1.2 manifest and artifact | `Manifest.decode`, `Artifact.decode`. Unknown fields ignored, a different `schema` major rejected, one unreadable message reported and skipped rather than failing the release |
| §1.3 integrity and signatures | `sha256Hex` over an artifact's exact bytes; `verifyManifestSignature` — **pure-Dart Ed25519** (`src/ed25519.dart`) over the **RFC 8785 (JCS)** form (`src/jcs.dart`) of the manifest without `signatures`, checked against the manifest's exact bytes. No keys configured means verification is off, which SPEC §1.3 permits and mobile OTA should not do |
| §2 endpoints | `Transport`, a one-function seam. `ioTransport()` in `package:glossa/io.dart` is the `dart:io` one; it sends `If-None-Match` and never a credential |
| §3 loading | `GlossaClient`: memory → persisted → network → bundled → inline, with artifact *bytes* taken from memory → persisted → bundled → network. Atomic activation, `ReleaseStore` for the last-good release, and hashing plus decode in an isolate |
| §4.1 negotiation | `canonicalizeLocale` (RFC 5646 §4.5), `lookupLocale` (RFC 4647 §3.4), `resolveLocales`, `acceptLanguage` |
| §4.2 fallback chain | `fallbackChain`: explicit edges depth-first, truncations, `"*"`, the source locale, cycles stopped at the first repeat |
| §4.3 message resolution | `Localizer.t` formats with the locale the message was **found** in |
| §5 formatting | `formatMessage` / `formatToParts`: a MessageFormat 2 interpreter over the precompiled data model. No parser. Formatting never throws |
| §6 observability | `explain()` returns the SPEC document verbatim; `GlossaClient.errors` is the error channel with the six types and 60-second repeat suppression, and it spans every release of a runtime's life. **No telemetry** |

```dart
import 'package:glossa/glossa.dart';
import 'package:glossa/io.dart';

final glossa = GlossaClient(
  edge: 'https://edge.example.com',
  deliveryKey: 'pk_live_…',          // publishable by design (SPEC §2)
  locales: ['de-AT', 'en'],
  publicKeys: [GlossaPublicKey.parse('k_2026a', '…')],
  transport: ioTransport(),
  store: FileReleaseStore.scoped(
    supportDirectory,                 // path_provider, in a Flutter app
    deliveryKey: 'pk_live_…',
    environment: 'production',
  ),
  bundled: await readBundledRelease(Directory('assets/glossa')),
);
glossa.errors.listen((e) => log.warning('$e'));
await glossa.ready;                   // never throws; never rejects

glossa.t('cart.checkout');                    // "Zur Kassa"
glossa.t('cart.items', values: {'n': 3});     // "3 Artikel"
glossa.direction;                             // Direction.ltr
glossa.explain('cart.checkout').chain;        // [de-AT, de, en]
glossa.explain('cart.checkout').source;       // Source.persisted
```

`Catalog.fromRelease` is still there for a host that already holds a release
and wants no loader at all.

### How the loader is safe

- **Atomic activation.** A release becomes visible only after its manifest has
  verified *and* every artifact its active fallback chain needs has loaded and
  verified. The switch is two field assignments with no `await` between them,
  so a reader sees the whole old release or the whole new one. Any failure
  leaves the previous release serving and goes to the error channel.
- **The manifest is written last.** `FileReleaseStore` writes every artifact
  first, each through a temporary file and a rename, and commits
  `manifest.json` last — the rule [`runtimes/go/store.go`](../go/store.go) set.
  Artifacts are content-addressed, so one written for a release that never
  activated is inert: nothing names it. A process killed at any point therefore
  leaves the previous manifest in place with all of its artifacts present, and
  a half-downloaded release can never be activated on the next start. The ETag
  lives in a sidecar that records the digest of the manifest it belongs to, so
  a torn write can never pair an ETag with a manifest it didn't come from.
- **Off the UI thread.** Hashing, parsing and building the data model are one
  pure function over sendable data, run through `Isolate.run` on the VM and
  AOT (so Flutter drops no frame) and inline on the web, which has no isolates.

## What is coming

- **Wave 3 — Flutter.** `package:glossa/flutter.dart`, a `GlossaText` that turns
  formatted parts into a `Text.rich`, and the
  [`markup.json`](../testdata/markup.json) safe-tag contract. Markup already
  reaches `parts()` as `MarkupPart`; nothing renders it yet.
- **Wave 3 — a web store.** `ReleaseStore` is an interface and
  `MemoryReleaseStore` and `FileReleaseStore` implement it; an IndexedDB one
  for Flutter web is still missing, so a web build keeps its release in memory
  and reloads it from the edge on every start.
- `glossa generate`'s typed accessors for Dart.
- Background refresh has the SPEC §3 timer but no app-resume or visibility
  hook; those belong with the Flutter layer.

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

# RFC 0005 §6.4: the core must build for the web, so no dart:io in it
dart compile js -o .dart_tool/web_compile.js tool/web_compile.dart
```

CI runs the same three commands on a pinned SDK in the `runtimes-dart` job.

| Suite | Source | Status |
|---|---|---|
| `test/scenarios_test.dart` | `runtimes/testdata/scenarios/*.json` | every case, **no skips** |
| `test/loading_test.dart` | `runtimes/testdata/loading/*.json` | every step of every sequence, **no skips** |
| `test/loader_test.dart` | the §3 cases a fixture can't ship: the bundle, the persisted store on disk, a torn write | — |
| `test/jcs_test.dart` | RFC 8785 §3.2.2–3.2.3 and Appendix B, plus number vectors from Node 22 (V8) | — |
| `test/ed25519_test.dart` | RFC 8032 §7.1, plus the rejections | — |
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
