# @glossa/unplugin

## 0.1.0

### Minor Changes

- [#30](https://github.com/felixgeelhaar/glossa/pull/30) [`5c5f18d`](https://github.com/felixgeelhaar/glossa/commit/5c5f18d22cd735a8789b5c0c0357d597454f5e62) Thanks [@felixgeelhaar](https://github.com/felixgeelhaar)! - New package: the bundler plugin that finds where every message is used (RFC 0004 §2.1), on `unplugin` for Vite, Rollup, webpack and esbuild. It reads modules after the framework transforms (Vue templates compiled, JSX and TS stripped), recognizes `t()`/`$t()`, `<T id>`, `<GlossaText id>`, `<glossa-*>` elements and the typed accessors from `glossa generate` with literal keys only, maps each hit back to the original file, line and code-point column through the combined source map, and names the component (SFC or `.astro` file, or the enclosing capitalized JSX function or class) and the route (Astro's `src/pages/**` or a `routes` option). It never makes network calls: the build writes `.glossa/usages.json` (`glossa.usages/v1`) beside its output for `glossa context push`, with the application, commit and branch from options, GitHub Actions or git. Passes the shared usage fixtures under real Vite, Astro, Rollup, esbuild and webpack builds.
