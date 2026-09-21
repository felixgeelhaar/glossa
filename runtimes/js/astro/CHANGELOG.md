# @glossa/astro

## 0.1.0

### Minor Changes

- [#30](https://github.com/felixgeelhaar/glossa/pull/30) [`b375c90`](https://github.com/felixgeelhaar/glossa/commit/b375c90be1c1d37a851eb20ee24b2d1146cb8625) Thanks [@felixgeelhaar](https://github.com/felixgeelhaar)! - `astro build` now records where messages are used: the integration adds `@glossa/unplugin` to Vite, which writes `.glossa/usages.json` beside `outDir` for `glossa context push`, with `.astro`/`.vue` file names as components, `src/pages/**` routes, and the release's message keys for typed accessors. The new `usages` option passes plugin options (`application`, `commit`, `branch`, `routes`, `keys`); `usages: false` turns it off.

### Patch Changes

- Updated dependencies [[`5c5f18d`](https://github.com/felixgeelhaar/glossa/commit/5c5f18d22cd735a8789b5c0c0357d597454f5e62)]:
  - @glossa/unplugin@0.1.0
