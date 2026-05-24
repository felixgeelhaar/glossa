# @glossa/cli

## 0.1.0

### Minor Changes

- [`5d7d5b6`](https://github.com/felixgeelhaar/glossa/commit/5d7d5b6c5503df68737813d86c1939bae61c547f) - Initial public release.
  - `@glossa/ui`: Lit design-system primitives + tokens (light/dark/system).
  - `@glossa/format`: ICU MessageFormat subset (variables, plurals, select, nesting) backed by `Intl.PluralRules`. Zero runtime deps.
  - `@glossa/sdk`: framework-agnostic HTTP fetch + in-memory bundle cache + SSE subscription.
  - `@glossa/elements`: `<glossa-provider>` + `<glossa-text|rich|plural|select>` web components.
  - `@glossa/cli`: build-time tooling — `glossa init / scan / pull / push`.

### Patch Changes

- Updated dependencies [[`5d7d5b6`](https://github.com/felixgeelhaar/glossa/commit/5d7d5b6c5503df68737813d86c1939bae61c547f)]:
  - @glossa/sdk@0.1.0
