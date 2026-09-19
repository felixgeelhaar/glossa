# @glossa/runtime

Glossa's JavaScript runtime. It implements the
[runtime and delivery contract](../../SPEC.md): it loads a signed release from
memory, persisted storage, the edge or the build's bundle, resolves the locale
and its fallback graph, formats with a **MessageFormat 2 interpreter over
precompiled messages**, and explains every decision. It runs on `Intl.*` and
WebCrypto only and has no dependencies. Framework adapters (Vue, React, web
components) build on it.

```ts
import { createRuntime, resolveLocales, navigatorLanguages } from "@glossa/runtime";

const glossa = createRuntime({
  edge: "https://edge.example.com",
  deliveryKey: "pk_7Hc2…", // publishable, read-only
  locales: resolveLocales(user?.locale, org?.locale, navigatorLanguages),
});

glossa.t("cart.checkout", {}, { default: "Zur Kasse" }); // usable immediately, never throws
await glossa.ready; // first load settled (persisted, then network)
glossa.t("cart.items", { count: 3 }); // "3 Artikel"
document.documentElement.dir = glossa.dir;
glossa.subscribe(render); // re-render after a new release or locale activates
```

With nothing but `edge` and `deliveryKey` it persists the last good release in
`localStorage`, refreshes every 5 minutes and when the page becomes visible,
and falls back along the manifest's fallback graph to the source locale, then
to the inline default, then to the message ID. Everything else is optional.

## `createRuntime(options?) → Runtime`

| Option | Default | |
|---|---|---|
| `edge`, `deliveryKey` | none | Edge origin and publishable key. Without them the runtime makes no requests. |
| `environment` | `"production"` | |
| `locales` | `navigator.languages` | Requested locales, most preferred first. Canonicalized (`en_us` → `en-US`, `iw` → `he`). |
| `bundled` | none | `{ manifest, artifacts }` from `glossa pull --release`, artifacts keyed by SHA-256. Renders synchronously at construction and is the last resort offline. |
| `publicKeys` | none | `[{ keyId, key }]`, base64url raw Ed25519. When set, manifests without a valid signature are rejected. |
| `storage` | `webStorage()` in browsers | Where last-good persists. `memoryStorage()`, `indexedDbStorage()` from `@glossa/runtime/idb`, your own `{ get, set }`, or `null` for none. |
| `transport` | `fetch` | Any `(url, { headers }) → Promise<{ status, headers.get, text() }>`. |
| `refreshInterval` | `300000` | Background manifest refresh in ms; `0` turns it off. The default timer is `unref`'d, so it never keeps a server process alive. |
| `timer` | `setInterval` | `(tick, ms) → cancel`, for tests or custom scheduling. |
| `bidiIsolation`, `functions` | MF2 defaults | Passed to the interpreter. |
| `onError` | none | Error channel listener (more with `runtime.onError`). |
| `errorInterval` | `60000` | An identical error is reported at most once per interval. |

The `Runtime`:

| Member | |
|---|---|
| `t(id, values?, { default? }) → string` | Renders `id` along the active chain. |
| `parts(id, values?, { default? }) → Part[]` | The same as parts (text, markup, bidi isolates, fallbacks, values), for adapters and typed accessors. |
| `explain(id, locales?) → Explanation` | SPEC §6, without side effects: `{ id, requested, locale, chain, resolvedFrom, release, source, steps }`. With `locales`, explains those instead of the active ones and loads nothing. |
| `locale`, `dir`, `release` | The active locale, its direction from the manifest, and `{ id, version }`. |
| `availableLocales` | The active release's `locales` (`{ code, direction }[]`, empty until one is active), e.g. for a locale picker. |
| `setLocales(locales) → Promise` | Switches once the new chain's artifacts are loaded. |
| `refresh() → Promise` | Revalidates now. Concurrent calls share one request. |
| `ready` | Settles after the first load. Never rejects. |
| `subscribe(fn)`, `onError(fn)` | Both return an unsubscribe function. Listener exceptions are contained. |
| `dispose()` | Stops the timer and the visibility listener and drops listeners. |

Errors are `{ type, detail, messageId?, locale?, releaseId? }` with `type` one of
`network`, `integrity`, `signature`, `schema`, `format`, `missing-message`.
Nothing is sent anywhere; the application decides what to do with them.

### Locale resolution

`resolveLocales(...resolvers)` runs a resolver chain in order and returns the
canonicalized, de-duplicated requested locales. A resolver is a value or a
function; one that throws is skipped. The contract's default chain is explicit
→ user → organization → request metadata → `Accept-Language` /
`navigator.languages` → source locale:

```ts
resolveLocales(params.lang, user?.locale, org?.locale, acceptLanguage(req.headers["accept-language"]));
```

The source locale is always the implicit last step. `lookupLocale`
(RFC 4647 Lookup), `fallbackChain` (SPEC §4.2), `canonicalLocales` and
`acceptLanguage` are exported for servers and tools.

### How loading behaves

- **Order** (SPEC §3): the release in memory, then persisted last-good, then the
  edge, then the bundle, then the inline default or message ID. Artifacts are
  content-addressed, so each one is looked up by its hash in memory, the
  persisted release, the bundle and only then the edge: a partly cached or
  bundled release only fetches what it has nowhere else, and only for the
  active fallback chain.
- **Atomic activation.** A manifest is checked (schema major version and
  environment first, then the signature), every artifact of the chain is verified against its
  SHA-256, and only then does the release switch, in one step. Until then the
  previous release keeps serving. Activations are serialized, so a locale
  switch during a release update can't resurrect the old release.
- **`explain().source`** is where the rendered text came from: the source a
  release was activated from, or `memory` once a later refresh brings nothing
  new (a `304`, or a failure). It is `inline` whenever the inline default or
  the message ID renders.
- **Bundled vs. persisted.** Persisted last-good wins, unless the bundle is a
  newer release (higher `release.version`), which happens after an app update
  while the edge is unreachable. Bundled artifacts are part of the app build
  and are trusted like its code: they aren't re-hashed or re-verified.
- **Persistence** stores the manifest, its ETag and the verified bytes of every
  artifact of that release the runtime has loaded, across restarts. Storage
  failures only cost persistence.
- **Bad messages.** A message in an artifact that isn't an MF2 data-model
  message is dropped with a `schema` error (with its `messageId`) and resolves
  as missing, so the fallback chain covers it; it never blocks the release.
- **Never blank.** A message that formats to the empty string renders the
  inline default, or the message ID.

## `format` and `formatToParts`

The interpreter is usable on its own:

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

What it implements:

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
  `bad-message`.

## Conformance

`pnpm test` runs:

- the **runtime contract fixtures** (`runtimes/testdata`, SPEC §7): every
  resolution scenario (negotiation, fallback graph and cycles, formatting with
  the locale a message was found in, missing messages) and every loading
  sequence (persisted last-good, atomic activation, signatures, schema version,
  cold offline start), through a fake edge and in-memory storage. A restart is
  a new runtime sharing the storage. No skips.
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

Minified and brotli-compressed, measured by `pnpm size` per import, so each
line is what an app that imports only that pays:

| Import | Size | Budget |
|---|---|---|
| `{ format, formatToParts }` (interpreter only) | 3.13 kB | 4 kB |
| `{ createRuntime }` (interpreter, loader, verification, resolver, `explain`) | 5.99 kB | 6.5 kB |
| `{ createRuntime, resolveLocales, acceptLanguage }` | 6.15 kB | 6.5 kB |
| `@glossa/runtime/idb` | 0.26 kB | 0.5 kB |

RFC 0002 §8 set 4 kB for the whole JS core. The interpreter alone fits it; the
contract's loader, SHA-256 and Ed25519 verification, JCS, the fallback graph,
persistence, background refresh and `explain` add about 2.9 kB. The 6.5 kB
budget keeps ~0.5 kB for namespace-level lazy loading (bundle splitting).
Framework adapters are separate packages with their own budgets. If the budget
gets tight, trim here before dropping contract behaviour: the `Intl.Locale`
script fallback in `dirOf` (only needed where `textInfo` is missing) and the
date/time option validation are the candidates.

## Platform requirements

WebCrypto `SHA-256` everywhere, and `Ed25519` only when `publicKeys` are set
(Chrome 137+, Firefox 129+, Safari 17+, Node 22+). Where Ed25519 is missing,
signature checks fail closed: the manifest is rejected and the last good release
keeps serving.

## Deliberate differences from the reference implementation

- Dotted variable names aren't looked up as paths into nested values
  (`{$user.name}` needs a `user.name` key). The reference does this as an
  extension; the spec doesn't.
- `timeZone=input` keeps the operand's zone, and converting between zones
  isn't reported as an error.
- `:number select=plural` means cardinal plural rules (the reference passes
  `type: "plural"` through to `Intl.PluralRules`, which rejects it).
- Custom functions return `toParts()` only; `format()` joins the parts.
