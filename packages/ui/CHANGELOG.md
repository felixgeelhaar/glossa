# @felixgeelhaar/glossa-ui

## 0.2.0

### Minor Changes

- [`ab66bb0`](https://github.com/felixgeelhaar/glossa/commit/ab66bb0fcdbe930d5ee2bb02c147d3480aba4c2f) - `<gl-tabs>` now supports an overflow `More ▾` group. Items can opt in with `group: "more"` and are rendered inside a popover menu instead of inline; the trigger reflects an aria-current state when the active tab lives inside it. Closes on Esc, click outside, or selection. Existing call sites (no `group`) keep the previous inline-only behavior.

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
