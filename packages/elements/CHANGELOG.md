# @felixgeelhaar/glossa-elements

## 0.3.0

### Minor Changes

- [#31](https://github.com/felixgeelhaar/glossa/pull/31) [`00d812b`](https://github.com/felixgeelhaar/glossa/commit/00d812b5f2f0bc6b105cdae10ddc52fb5a540cff) Thanks [@felixgeelhaar](https://github.com/felixgeelhaar)! - `<glossa-text>`, `<glossa-rich>`, `<glossa-plural>` and `<glossa-select>` accept `message="…"` as well as `key="…"`. Vue reserves `key` for its own reconciliation and never renders it as a DOM attribute, so inside `.vue` templates `<glossa-text key="…">` reached the element without a message ID and always showed its slot fallback, whatever the locale. Use `message` in Vue templates; `key` keeps working everywhere else, and `message` wins when both are set.

## 0.1.3

### Patch Changes

- Updated dependencies [[`9a30270`](https://github.com/felixgeelhaar/glossa/commit/9a30270dc10ffd248996a6db400d3a804e7e4105)]:
  - @felixgeelhaar/glossa-sdk@0.2.0

## 0.1.2

### Patch Changes

- [`6ac25f0`](https://github.com/felixgeelhaar/glossa/commit/6ac25f05ceac380c396fa667e46349988ea6b37a) - `<glossa-text>` now surfaces a hydration state on the host element so SSR fallback content is visually distinct from the live-resolved value: `aria-busy="true"` + `data-glossa-pending` while the provider's first bundle is in flight, `data-glossa-missing` when the key is genuinely absent post-hydration. Default styles dim pending content slightly and dotted-outline missing content; consumers can override via `::slotted()` selectors on their own page CSS.

## 0.1.1

### Patch Changes

- Rewrite package READMEs to describe the actual shipped implementation. The 0.1.0 versions inherited placeholder "Stub. Implementation lands…" READMEs from the planning phase, which made npm show every package as empty.

- Updated dependencies []:
  - @felixgeelhaar/glossa-format@0.1.1
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
  - @felixgeelhaar/glossa-format@0.1.0
  - @felixgeelhaar/glossa-sdk@0.1.0
