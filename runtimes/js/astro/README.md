# @glossa/astro

Glossa for Astro 5. Static pages are rendered with a release at build time, so
they ship translated HTML rather than client JavaScript. Vue islands and
`<glossa-*>` elements share one runtime per page, which starts from the same
release as the server. Locale routing follows Astro's i18n routing.

```js
// astro.config.mjs
import vue from "@astrojs/vue";
import { defineConfig } from "astro/config";
import glossa from "@glossa/astro";

export default defineConfig({
  i18n: { locales: ["de", "en"], defaultLocale: "de" },
  integrations: [
    vue({
      appEntrypoint: "@glossa/astro/vue",
      template: { compilerOptions: { isCustomElement: (tag) => tag.startsWith("glossa-") } },
    }),
    glossa({ edge: "https://edge.example.com", deliveryKey: "pk_7Hc2…", elements: true }),
  ],
});
```

## Options

| Option | Default | |
|---|---|---|
| `edge`, `deliveryKey` | none | The build fetches the edge's current release once at build start. Islands and elements refresh from the edge in the browser. |
| `environment` | `production` | The release has to be for this environment. |
| `publicKeys` | none | Trusted signing keys, `[{ keyId, key }]`. |
| `release` | the edge's | A `glossa pull --release` directory (relative to the project root), or a `{ manifest, artifacts }` object. A directory holds `manifest.json` and `a/<sha256>.json`, which mirror the edge's paths. |
| `locales`, `defaultLocale` | Astro's `i18n`, then the release | The locales the site renders. |
| `prerender` | `"static"` | Controls rendering of `<glossa-*>` elements into the HTML: `"static"` does it on prerendered pages; `"all"` also does it on on-demand pages, which buffers them instead of streaming; `false` turns it off. |
| `inline` | `"auto"` | Controls inlining of the page locale's release slice, the manifest plus that locale's fallback-chain artifacts, as `<script type="application/json" id="glossa-release">`: `"auto"` inlines it on pages that have islands or providers, `"always"` on every page, `"never"` nowhere. |
| `elements` | `false` | Defines the elements on every page and makes `<glossa-provider>` without `edge` use the page runtime. |
| `usages` | on | Where messages are used. `astro build` runs [`@glossa/unplugin`](../unplugin), which writes `.glossa/usages.json` beside `outDir` (not inside it, so it's never deployed) for `glossa context push`. The object goes to the plugin (`application`, `commit`, `branch`, `routes`, `keys`, which defaults to the release's message keys); `false` turns it off. Components are the `.astro`/`.vue` file names; routes come from `src/pages/**`. |

The release is loaded through `@glossa/runtime`, so the build verifies it the
same way a browser would: schema, environment, the signature when keys are
set, and every artifact's SHA-256. If the release can't be loaded completely,
**the build fails**, because a static site must not quietly ship its inline
defaults. With no release configured, the build logs a warning and pages
render their inline defaults.

## In `.astro` components: `@glossa/astro/server`

```astro
---
import { alternates, getGlossa, localizePath } from "@glossa/astro/server";
const { t, locale, dir } = getGlossa(Astro);
---
<html lang={locale} dir={dir}>
  <head>
    <title>{t("home.title", {}, { default: "Willkommen" })}</title>
    {alternates(Astro).map((a) => <link rel="alternate" hreflang={a.hreflang} href={a.href} />)}
  </head>
  <a href={localizePath("/pricing", "en")}>English</a>
</html>
```

| | |
|---|---|
| `getGlossa(Astro)` | Returns `{ t, parts, explain, locale, dir, release, runtime }` for the page's locale. The page locale is `Astro.currentLocale` when the site renders it, otherwise the default locale. |
| `localizePath(path, locale)` | Gives `path` in `locale` under Astro's routing: the default locale is unprefixed unless `prefixDefaultLocale` is set, and other locales get their `/{path}/` prefix, with `base` respected. |
| `alternates(Astro)` | Returns hreflang targets for the current page, including `x-default`. |
| `localeParams()` | Returns `getStaticPaths()` entries for a `[...locale]` route. |
| `siteRouting` | The resolved locales, default locale and prefix setting. |

## Elements, islands and hydration

- **`<glossa-*>` elements** in `.astro` files, `.vue` templates or anywhere
  else in the page are rendered at build time by the middleware, which uses
  `prerender` from `@glossa/elements/ssr`. The HTML is translated before any
  JavaScript runs. Elements whose message is missing keep their inline
  default. Inside `.vue` files, write `message="…"` in place of `key="…"`,
  because Vue never renders `key` (see the elements' MIGRATION.md).
- **Vue islands** get the page's runtime through the app entrypoint
  `@glossa/astro/vue` (with your own entrypoint, call it from there). On the
  server that runtime is the request's, in the page's locale. In the browser
  it's one runtime per page (`getRuntime()` from `@glossa/astro/client`),
  created from the inlined slice. The first render therefore matches the
  server HTML, and a newer release from the edge activates afterwards.
  Prerendered elements carry `data-allow-mismatch`, so Vue doesn't report
  their translated text as a hydration mismatch.
- **Client JavaScript** never contains the release. It lives in the server
  bundle and in the per-page inline JSON.
- **On-demand (SSR) pages** keep streaming. The inline slice is inserted
  before `</body>` as the page streams through. Elements on those pages are
  only prerendered with `prerender: "all"`. For streamed pages, use
  `getGlossa()` and Vue components instead.

## Tests

`pnpm test` runs the pure parts (routing, page rendering, streaming inline,
release loading, the integration hooks) and one real `astro build` of
`test/fixture`: a static site with i18n routing, a Vue island, elements, and a
page without islands. The build takes about 15 s. It checks the translated HTML
per locale, the islands' server rendering, the inline slice, that no
client chunk contains the catalog, and the usages `@glossa/unplugin` wrote. Run `pnpm build` first, because the fixture
uses `dist/` the way an installed package would.

## Size

`@glossa/astro/client`, the only part of this package that ships to
browsers, is 0.56 kB brotli without the runtime (budget 0.75 kB). Islands also
load `@glossa/vue` (1.2 kB) and the runtime (6 kB); elements load
`@glossa/elements`.
