# @felixgeelhaar/glossa-sdk

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
