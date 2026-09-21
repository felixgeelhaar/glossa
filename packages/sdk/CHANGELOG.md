# @felixgeelhaar/glossa-sdk

## 0.3.0

### Minor Changes

- [#29](https://github.com/felixgeelhaar/glossa/pull/29) [`1020fa2`](https://github.com/felixgeelhaar/glossa/commit/1020fa2c27b7adffd6f863a9fde8e1161672a8a6) Thanks [@felixgeelhaar](https://github.com/felixgeelhaar)! - Locales are full BCP 47 tags now. `describeLocale(tag)` returns the canonical code, the explicit language, script and region subtags, and the text direction (`ltr`/`rtl`, from the explicit or CLDR likely script). The `Locale` wire type describes `GET /locales` items, which now carry the same fields. The bundle cache keys on the canonical code the server returns, so a bundle requested as `de-de` is also found as `de-DE` and patched by SSE events. `TranslationStatus` now includes `ai_translated`, which the server has been sending since 0.2.

## 0.2.0

### Minor Changes

- [`9a30270`](https://github.com/felixgeelhaar/glossa/commit/9a30270dc10ffd248996a6db400d3a804e7e4105) - Add `resolveApiError(payload, opts?)` plus the matching `ApiErrorBody`, `ApiErrorPayload`, and `ResolveOptions` types. Takes the JSON envelope emitted by glossa-aware Go backends via the new `github.com/felixgeelhaar/glossa/apierr` Go module (`{ error: { code, message, key, params?, status } }`), looks the `key` up in a provided messages map, and renders the result via the existing glossa-format interpolator. Falls back gracefully to the server-supplied English `message` on bundle miss, to the legacy `{ error: "literal" }` shape, and to `"Unknown error"` on malformed input — never throws so it's safe to call from a failing-fetch path.

## 0.1.1

### Patch Changes

- Rewrite package READMEs to describe the actual shipped implementation. The 0.1.0 versions inherited placeholder "Stub. Implementation lands…" READMEs from the planning phase, which made npm show every package as empty.

## 0.1.0

### Minor Changes

- [`5d7d5b6`](https://github.com/felixgeelhaar/glossa/commit/5d7d5b6c5503df68737813d86c1939bae61c547f) - Initial public release.
  - `@felixgeelhaar/glossa-ui`: Lit design-system primitives + tokens (light/dark/system).
  - `@felixgeelhaar/glossa-format`: ICU MessageFormat subset (variables, plurals, select, nesting) backed by `Intl.PluralRules`. Zero runtime deps.
  - `@felixgeelhaar/glossa-sdk`: framework-agnostic HTTP fetch + in-memory bundle cache + SSE subscription.
  - `@felixgeelhaar/glossa-elements`: `<glossa-provider>` + `<glossa-text|rich|plural|select>` web components.
  - `@felixgeelhaar/glossa-cli`: build-time tooling — `glossa init / scan / pull / push`.
