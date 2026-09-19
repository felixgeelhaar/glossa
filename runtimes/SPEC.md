# Glossa runtime and delivery contract (v1)

**Status:** Accepted — 2026-09-19, clarified after the first two implementations (JS, Go) · **Implements:** RFC 0002 §7–§8, intent §13, §33–§39, §51

This is the contract between the delivery plane (the Release context and `glossa-edge`) and every runtime (JS, Go, and later Dart, Swift, Kotlin). A runtime is conformant when it passes the scenarios in [`testdata/`](./testdata) and follows the MUST rules below. The words MUST, SHOULD and MAY are used as in RFC 2119.

## 1. Artifacts

### 1.1 Manifest

One manifest per (project, environment). It names the release currently served and every artifact in it. Schema: [`testdata/schemas/manifest.schema.json`](./testdata/schemas/manifest.schema.json).

```json
{
  "schema": "glossa.manifest/v1",
  "project": "prj_7Hc2…",
  "environment": "production",
  "release": { "id": "rel_9Qx…", "version": 42, "createdAt": "2026-09-19T08:00:00Z" },
  "sourceLocale": "de",
  "locales": [
    { "code": "de", "direction": "ltr" },
    { "code": "en", "direction": "ltr" },
    { "code": "ar", "direction": "rtl" }
  ],
  "fallback": { "de-AT": ["de"], "en-GB": ["en"], "*": ["en"] },
  "artifacts": {
    "de": { "default": { "sha256": "9f2c…", "size": 18342 } },
    "en": { "default": { "sha256": "41aa…", "size": 17110 } }
  },
  "signatures": [{ "keyId": "k_2026a", "alg": "Ed25519", "sig": "base64url…" }]
}
```

- `locales[].code` MUST be canonical BCP 47 (RFC 5646 §4.5, extensions and private use excluded), exactly as the platform stores it.
- `fallback` maps a locale to its ordered fallback locales. `"*"` is the default chain for any locale without its own entry. Fallback is a graph (intent §39): entries MAY chain (`de-AT → de-CH → de`), and runtimes MUST detect cycles and stop at the first repeat.
- `artifacts[locale][namespace]` names one artifact by the SHA-256 of its exact bytes. The `default` namespace always exists for every listed locale, even if empty. More namespaces arrive with bundle splitting (RFC 0002 §8). Until namespace routing is specified, runtimes load and merge **all** namespaces of each locale in the chain.
- Unknown top-level fields MUST be ignored. A different `schema` major version MUST be rejected, keeping the last good release (§3).

### 1.2 Artifact

One artifact holds one namespace of one locale. Schema: [`testdata/schemas/artifact.schema.json`](./testdata/schemas/artifact.schema.json).

```json
{
  "schema": "glossa.artifact/v1",
  "locale": "de",
  "namespace": "default",
  "messages": {
    "cart.checkout": { "type": "message", "declarations": [], "pattern": ["Zur Kasse"] }
  }
}
```

- `messages` values MUST be MessageFormat 2 data-model messages exactly as defined by [`messageformat/testdata/unicode/data-model/message.schema.json`](../messageformat/testdata/unicode/data-model/message.schema.json). They are precompiled, so runtimes contain no parser.
- A message ID is a dotted path of `[a-z0-9_-]` segments (`checkout.payment.submit`).
- A locale's artifact contains only the messages translated *for that locale*. Filling gaps is the runtime's fallback job, so a partly translated locale is never padded with source text.

### 1.3 Integrity and signatures

- Runtimes MUST verify an artifact's bytes against its manifest `sha256` before using it, and MUST discard it on mismatch.
- `signatures[].sig` is an Ed25519 signature over the **RFC 8785 (JCS) canonicalization** of the manifest with the `signatures` member removed.
- A runtime configured with one or more public keys MUST reject a manifest without a valid signature from one of them. A runtime without configured keys MAY skip signature verification; TLS still protects the transport. Server-side runtimes (Go) and mobile OTA SHOULD be configured with keys.

## 2. Delivery endpoints (`glossa-edge`)

| Request | Response | Caching |
|---|---|---|
| `GET /v1/{deliveryKey}/{environment}/manifest.json` | the manifest | `Cache-Control: public, max-age=60, stale-while-revalidate=300, stale-if-error=86400`; strong `ETag` (manifest digest); `304` on `If-None-Match` |
| `GET /v1/{deliveryKey}/a/{sha256}.json` | artifact bytes | `Cache-Control: public, max-age=31536000, immutable` |

- `deliveryKey` is a **publishable** key: public by design (it ships in browser bundles), scoped to one project, read-only, revocable, and it never grants access to the control plane. A revoked or unknown key answers `404`, never `401`, so key validity can't be probed separately from existence.
- The edge serves only what's in object storage. It has no database and no control-plane dependency (RFC 0002 §3).
- Responses carry `Access-Control-Allow-Origin: *`. The artifacts are public, and credentials are never used.

## 3. Loading (reliability order)

A runtime resolves the content it renders from the first available source (intent §51):

1. **Memory**: the release already loaded in this process or page.
2. **Persisted last-good**: the most recent release that loaded and verified completely, including its manifest and every artifact it used. Browser: IndexedDB, or `localStorage` for small catalogs. Go: a cache directory. Mobile: app storage.
3. **Network**: the edge. Revalidate the manifest with `If-None-Match`. Fetch only the artifacts for the locales in the active fallback chain, and only when their `sha256` isn't already cached.
4. **Bundled**: artifacts shipped with the build (`glossa pull --release`), for offline-first apps and cold starts. The bundle layout is fixed so every runtime and the CLI agree: a directory holding `manifest.json` (the manifest exactly as the edge serves it) and `a/<sha256>.json` for each artifact (exact bytes), i.e. the edge's URL space below `/v1/{deliveryKey}/`.
5. **Inline default**: the text the developer wrote at the call site (`<glossa-text key="…">Zur Kasse</glossa-text>`, or the Go `glossa.Default("…")` option). If there's none, the message ID itself, so a missing string is visible and never blank.

**Artifacts are content-addressed**, so any source holding bytes with the manifest's hash is equivalent. Runtimes look for an artifact in memory → persisted cache → bundled → network, and only go to the network for hashes they have nowhere else.

**Bundled vs persisted releases.** When both exist at startup, the one with the higher `release.version` wins, because an app update may ship a newer catalog than the one persisted. Bundled artifacts and manifests are trusted like application code and aren't re-hashed or signature-checked. Persisted ones are verified again when loaded.

Rules:
- A new release becomes active **atomically**: only after its manifest has verified and every artifact needed for the active chain has loaded and verified. Until then, the previous release keeps serving. A half-updated release MUST never be visible.
- A failure at any step falls through to the next step. It never throws into application code and never renders an empty string. Errors go to an observable error channel (§6).
- A manifest whose `environment` differs from the configured environment MUST be rejected (`schema` error).
- One message that can't be read (not a valid data-model message) is reported as a `schema` error with its `messageId` and resolves as missing. It doesn't block the release.
- A rendered result that is the empty string falls through to the inline default or the message ID; runtimes never render `""` for a message that isn't intentionally empty in the source locale.
- Runtimes SHOULD refresh the manifest in the background (default every 5 minutes, and on page visibility or app resume). Long-lived clients MAY also react to the control plane's `release.published` event where they're connected to it (development and preview only).

## 4. Locale resolution and fallback

### 4.1 Negotiation

The application supplies an ordered list of requested locales from a **resolver chain** (intent §13). The default chain is: explicit → user preference → organization preference → request metadata → `Accept-Language` / `navigator.languages` → the manifest's `sourceLocale`. Each resolver is optional and pluggable. Its output is **canonicalized** before matching (`en_us` → `en-US`, `iw` → `he`).

The **active locale** is chosen by RFC 4647 §3.4 *Lookup* over the manifest's `locales`. For each requested tag in order, try the tag, then progressively truncate it (`zh-Hant-TW` → `zh-Hant` → `zh`, dropping a trailing single-character subtag together with the one before it). The first available tag wins. If nothing matches, use `sourceLocale`.

### 4.2 Fallback chain

The chain for the active locale `L` is built like this, dropping duplicates and stopping on cycles:

1. `L`
2. `fallback[L]`, expanded depth-first in order: each entry is followed by *its own* `fallback` entries before the next one. Recursion follows explicit edges only, not truncation or `"*"`.
3. only if `L` has no `fallback` entry: the truncations of `L` that are available locales (`fr-CA` → `fr`)
4. `fallback["*"]`, expanded the same way as step 2
5. `sourceLocale`

### 4.3 Message resolution

To render message `id`, walk the chain and use the first locale whose loaded artifact contains `id`. Format that message **with the locale it was found in** (plural rules and number formats must match the language of the text). If no locale in the chain contains `id`, use the inline default (§3.5).

## 5. Formatting

- Runtimes format with an MF2 interpreter over the data model, as `@glossa/runtime` does. They MUST pass the runtime cases of the Unicode MessageFormat suite and `messageformat/testdata/glossa/runtime-format.json` (implementation-defined outputs excepted, and documented).
- Formatting MUST NOT throw. A failing expression renders its MF2 fallback representation (`{$name}`), and the error is reported (§6).
- Bidi isolation is on by default, per the MF2 spec. The active locale's `direction` from the manifest is exposed to the application, so it can set `dir`.

## 6. Observability

Every runtime exposes:

- `explain(id, locales?)` returns, without side effects:
  ```json
  {
    "id": "cart.checkout",
    "requested": ["de-AT", "en"],
    "locale": "de-AT",
    "chain": ["de-AT", "de", "en"],
    "resolvedFrom": "de",
    "release": { "id": "rel_9Qx…", "version": 42 },
    "source": "persisted",
    "steps": [
      { "locale": "de-AT", "outcome": "missing" },
      { "locale": "de", "outcome": "found" }
    ]
  }
  ```
  - `resolvedFrom` is `null` when the inline default was used.
  - `source` is where the **active release** was loaded from: `network`, `persisted` or `bundled`. It becomes `memory` once a later refresh brings nothing new (a `304` or a failure). The startup load (construction plus the first refresh) keeps its original source. It's `inline` whenever the inline default or the message ID is rendered.
  - `steps[].outcome` is `found`, `missing` or `not-loaded` (the locale's artifacts aren't loaded, e.g. `explain` for locales other than the active chain).
  - Fallback targets that aren't in `manifest.locales` stay in the chain and resolve as `missing`.
- An **error channel** for load, verification and format errors. Each error is `{ type, detail, messageId?, locale?, releaseId? }`, with types `network`, `integrity`, `signature`, `schema`, `format` and `missing-message`. Runtimes MUST rate-limit repeats of the same error: errors are the same when all five fields are equal, and a repeat within 60 s is dropped. `missing-message` is reported only while a release is active; a cold start without any release reports `network` once, not one error per message.
- Runtimes MUST NOT send telemetry anywhere unless the application explicitly enables it (intent §17).

## 7. Conformance fixtures

[`testdata/scenarios/*.json`](./testdata/scenarios) each describe a manifest, its artifacts (inline) and a list of cases:

```json
{
  "description": "de-AT falls back to de, then en",
  "manifest": { … },
  "artifacts": { "<sha256>": { … artifact … } },
  "cases": [
    { "requested": ["de-AT"], "id": "cart.checkout", "values": {},
      "exp": "Zur Kasse", "expLocale": "de-AT",
      "expChain": ["de-AT", "de", "en"], "expResolvedFrom": "de" }
  ]
}
```

Loading-order behaviour (persisted last-good, atomic activation, integrity failure, signature rejection, schema version, cold offline start) is covered by `testdata/loading/*.json`. Each file lists a sequence of edge responses and the expected active release after each one. Runtimes drive these through a fake transport. Field reference: [`testdata/README.md`](./testdata/README.md).

Every runtime runs both suites in CI. A bug found in any runtime becomes a new case here first.

## 8. Versioning

This contract is versioned by the `schema` fields (`glossa.manifest/v1`, `glossa.artifact/v1`). Additive, optional fields are minor changes. Anything a v1 runtime would misread needs `v2`. The edge MUST then serve both versions until no active runtime asks for v1, which is visible in edge metrics by requested path.
