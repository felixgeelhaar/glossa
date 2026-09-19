# @glossa/unplugin

Finds where every message is used in a web build: file, line, column,
component and route. One core on [`unplugin`](https://github.com/unjs/unplugin)
serves Vite, Rollup, webpack and esbuild; Astro gets it from
[`@glossa/astro`](../astro). The build writes `.glossa/usages.json`
(`glossa.usages/v1`, [RFC 0004 §2](../../../docs/rfcs/0004-context.md)) and
`glossa context push .glossa/usages.json` uploads it. That's how a
translator learns where a string appears.

The plugin **never makes network calls** and never changes your code. It
needs no credentials, and builds stay reproducible.

```ts
// vite.config.ts
import { readFileSync } from "node:fs";
import vue from "@vitejs/plugin-vue";
import glossa from "@glossa/unplugin/vite";

export default defineConfig({
  plugins: [
    vue(),
    glossa({
      application: "web",
      keys: () => Object.keys(JSON.parse(readFileSync("locales/en.json", "utf8"))),
      routes: { "/checkout/payment": ["src/checkout/**"] },
    }),
  ],
});
```

```jsonc
// .glossa/usages.json
{
  "schema": "glossa.usages/v1",
  "application": "web",
  "commit": "9f2c1e7…", "branch": "feat/checkout-copy",
  "tool": { "name": "@glossa/unplugin", "version": "0.1.0" },
  "usages": [
    { "key": "checkout.pay", "file": "src/checkout/PaymentFooter.vue", "line": 42, "column": 9,
      "component": "PaymentFooter", "kind": "t" }
  ]
}
```

The other bundlers import from `@glossa/unplugin/rollup`,
`@glossa/unplugin/webpack` and `@glossa/unplugin/esbuild`.

## Options

| Option | Default | |
|---|---|---|
| `application` | the root package.json's name as a slug (`@acme/web` → `web`) | The application's slug in the Glossa project. |
| `commit` | `GITHUB_SHA`, else `git rev-parse HEAD` | Full commit ID. On `pull_request` events `GITHUB_SHA` is the merge commit; pass `github.event.pull_request.head.sha` to tie usages to the head. |
| `branch` | `GITHUB_HEAD_REF`, else `GITHUB_REF_NAME` (branch refs), else the checked-out branch | Short branch name. |
| `outDir` | `.glossa/` beside the build output (`dist/` → `.glossa/`) | Where `usages.json` goes, relative to `root`. Keep it outside anything you serve. |
| `root` | Vite's `root`, webpack's `context`, esbuild's `absWorkingDir`, else the working directory | `file` paths are relative to it. Files outside it and in `node_modules` aren't reported. |
| `keys` | none | The catalog's message keys, or a function returning them. They name the typed accessors from `glossa generate`; without them accessor calls aren't reported, every other shape is. |
| `routes` | none | Route pattern → root-relative globs (`**` crosses directories, `*` doesn't). The first matching entry names a usage's route. Astro's `src/pages/**` routes need no entry. |

When the build can't be named (no application, commit or branch, say in a
tarball without git), the plugin warns and writes nothing: the document
requires all three.

## What counts as a usage

The contract is the shared fixture suite in
[`runtimes/testdata/usages`](../../testdata/usages/README.md), which
`glossa extract` passes too. In short:

- `t("…")`, `$t("…")` and any `.t("…")`/`.$t("…")` call (kind `t`)
- `<T id="…">` (React) and `<GlossaText id="…">` (Vue) (kind `component`)
- `<glossa-text|rich|plural|select>` with `key="…"` or `message="…"`; `key` wins (kind `element`)
- the typed accessors `glossa generate` writes: `m.checkout.pay(…)` (kind `accessor`)

Only literal keys count, and only valid message keys. A variable, a
concatenation or a template literal with `${}` is dynamic and never
reported, so unused messages are reported, never deleted. Comments,
strings, JSX and HTML text, other components and generated files
(`// Code generated … DO NOT EDIT.` on the first line) never count.

- **Component**: the `.vue` or `.astro` file name, or for JSX/TSX the
  nearest enclosing capitalized function (declaration, named function
  expression, or arrow assigned to a `const`) or class.
- **Route**: Astro's `src/pages/**` path, else the `routes` option.
- **Position**: 1-based line, and a 1-based column in Unicode code points
  that points at the first character of the key inside its quotes (an
  accessor's first path segment).
- **Order**: by key, file, line and column, strings by code point.

## How it works, per bundler

The plugin reads each module **after** the framework transforms, so it sees
what the bundler sees: Vue templates as render functions, `.astro` markup as
`$$renderComponent` calls, JSX as `jsx()`/`createElement()` calls, TypeScript
stripped. It parses that code into an ESTree AST, finds the call shapes
above, and maps every hit back to the original file through the module's
source map. Source maps are only as fine as their producers make them (Vue
maps a template expression, not the string inside it), so the mapped
position is then pinned to the key itself in the original text: the exact
spot when the producer copied the text verbatim, else the nearest matching
key after the mapped position. Each original position is used once per
module.

| Bundler | Runs | Parser | Source map |
|---|---|---|---|
| Vite (and Astro) | `enforce: "post"`, builds only (`apply: "build"`) | Vite's `this.parse` | `this.getCombinedSourcemap()`. The plugin asks `@vitejs/plugin-vue` for SFC source maps even when `build.sourcemap` is off; this doesn't change the bundle. HTML entries are read in `transformIndexHtml`, as written. |
| Rollup | where you list it: put it **after** the plugins that compile TS, JSX or SFCs | Rollup's `this.parse` | `this.getCombinedSourcemap()` |
| webpack 5 | as a post loader, after your loaders | acorn | the incoming loader map (the plugin still works when a loader emits none, by searching from the same line) |
| esbuild | its own `onLoad`, which returns nothing: list it **first** | acorn | esbuild plugins see files before esbuild compiles them, so the plugin strips TS/JSX with esbuild's `transform` and reads that map. `.vue`/`.astro` files need Vite. |

Only Vite reads HTML files. The usages of every build environment (Astro's
server, prerender and client builds, Vite's SSR build) are merged into one
document, which is rewritten after each bundle is written. Separate
`vite build` processes (say, client then `--ssr`) don't share one: the
last overwrites it, so give each its own `outDir` and push both.

## Not in this package yet

The in-product editor's overlay loader (RFC 0004 §5.1) joins this plugin
later, behind an `environment` option that is never `production`.
Per-route chunking waits for namespace routing in the SPEC.

## Development

```bash
pnpm --filter @glossa/unplugin test
```

`test/fixtures.test.ts` runs every fixture case marked for `unplugin`
through a real `vite build` (and `vite build --ssr` for Vue, `astro build`
for Astro), the TS/TSX cases also through Rollup, esbuild and webpack, and
checks each `usages.json` against the JSON Schema and `expected.json`.
