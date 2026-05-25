# @felixgeelhaar/glossa-cli

## 0.2.0

### Minor Changes

- [`a931d3b`](https://github.com/felixgeelhaar/glossa/commit/a931d3be960fa0ba9ef6efa799bb3b29b08d29f4) - `glossa init` is now interactive by default — prompts for API URL, project slug, locales, and API key. The previous non-interactive path remains available via `glossa init --yes` (CI-friendly) or programmatic callers that pass flags directly. Auto-skips prompts when stdin isn't a TTY so existing scripts keep working without a flag flip.

## 0.1.1

### Patch Changes

- Rewrite package READMEs to describe the actual shipped implementation. The 0.1.0 versions inherited placeholder "Stub. Implementation lands…" READMEs from the planning phase, which made npm show every package as empty.

- Updated dependencies []:
  - @felixgeelhaar/glossa-sdk@0.1.1

## 0.1.0

### Minor Changes

- [`5d7d5b6`](https://github.com/felixgeelhaar/glossa/commit/5d7d5b6c5503df68737813d86c1939bae61c547f) - Initial public release.
  - `@felixgeelhaar/glossa-ui`: Lit design-system primitives + tokens (light/dark/system).
  - `@felixgeelhaar/glossa-format`: ICU MessageFormat subset (variables, plurals, select, nesting) backed by `Intl.PluralRules`. Zero runtime deps.
  - `@felixgeelhaar/glossa-sdk`: framework-agnostic HTTP fetch + in-memory bundle cache + SSE subscription.
  - `@felixgeelhaar/glossa-elements`: `<glossa-provider>` + `<glossa-text|rich|plural|select>` web components.
  - `@felixgeelhaar/glossa-cli`: build-time tooling — `glossa init / scan / pull / push`.

### Patch Changes

- Updated dependencies [[`5d7d5b6`](https://github.com/felixgeelhaar/glossa/commit/5d7d5b6c5503df68737813d86c1939bae61c547f)]:
  - @felixgeelhaar/glossa-sdk@0.1.0
