---
"@glossa/astro": minor
---

`astro build` now records where messages are used: the integration adds `@glossa/unplugin` to Vite, which writes `.glossa/usages.json` beside `outDir` for `glossa context push`, with `.astro`/`.vue` file names as components, `src/pages/**` routes, and the release's message keys for typed accessors. The new `usages` option passes plugin options (`application`, `commit`, `branch`, `routes`, `keys`); `usages: false` turns it off.
