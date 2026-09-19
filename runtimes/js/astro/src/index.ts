/**
 * `glossa()`, the Astro integration:
 *
 * ```js
 * // astro.config.mjs
 * import vue from "@astrojs/vue";
 * import glossa from "@glossa/astro";
 *
 * export default defineConfig({
 *   i18n: { locales: ["de", "en"], defaultLocale: "de" },
 *   integrations: [
 *     vue({ appEntrypoint: "@glossa/astro/vue" }),
 *     glossa({ edge: "https://edge.example.com", deliveryKey: "pk_…", elements: true }),
 *   ],
 * });
 * ```
 *
 * At build start it loads one release (a `glossa pull --release` directory,
 * or the edge's current release) and every page renders with it: `.astro`
 * components through `getGlossa(Astro)`, `<glossa-*>` elements prerendered by
 * the middleware, Vue islands through the shared runtime. Static pages ship
 * translated HTML; islands and elements hydrate from the inlined slice of the
 * same release, then refresh from the edge.
 */
import { resolve } from "node:path";
import { fileURLToPath } from "node:url";
import type { AstroConfig, AstroIntegration } from "astro";
import type { BundledRelease } from "@glossa/runtime";

import type { GlossaAstroOptions, PublicConfig } from "./config.js";
import { loadRelease } from "./release.js";
import { siteLocales } from "./routing.js";
import type { Routing } from "./routing.js";

export type { GlossaAstroOptions, PublicConfig } from "./config.js";
export { fetchRelease, loadRelease, readRelease } from "./release.js";
export type { ReleaseSource } from "./release.js";
export type { Routing, SiteLocale } from "./routing.js";

/** The site's locales and routing: the integration's options, else Astro's i18n, else the release's. */
export function resolveRouting(
  o: GlossaAstroOptions,
  config: Pick<AstroConfig, "i18n" | "base">,
  release: BundledRelease | undefined,
): Routing {
  const i18n = config.i18n;
  const locales = siteLocales(
    o.locales ?? i18n?.locales ?? release?.manifest.locales.map((l) => l.code) ?? [],
  );
  const defaultLocale =
    o.defaultLocale ??
    i18n?.defaultLocale ??
    release?.manifest.sourceLocale ??
    locales[0]?.code ??
    "en";
  const r = i18n?.routing;
  const prefixDefaultLocale = typeof r === "object" && !!r.prefixDefaultLocale;
  return { locales, defaultLocale, prefixDefaultLocale, base: config.base };
}

/** Serves `virtual:glossa/config` (public) and `virtual:glossa/release` (server-only). */
function virtualModules(values: Record<string, unknown>) {
  return {
    name: "@glossa/astro:virtual",
    resolveId: (id: string) => (id in values ? `\0${id}` : undefined),
    load: (id: string) =>
      id.startsWith("\0") && id.slice(1) in values
        ? `export default ${JSON.stringify(values[id.slice(1)])};`
        : undefined,
  };
}

export default function glossa(options: GlossaAstroOptions = {}): AstroIntegration {
  return {
    name: "@glossa/astro",
    hooks: {
      "astro:config:setup": async ({
        config,
        updateConfig,
        addMiddleware,
        injectScript,
        logger,
      }) => {
        const environment = options.environment ?? "production";
        const src =
          typeof options.release === "string"
            ? resolve(fileURLToPath(config.root), options.release)
            : options.release;
        const release = await loadRelease(src, {
          edge: options.edge,
          deliveryKey: options.deliveryKey,
          environment,
          publicKeys: options.publicKeys,
        });
        if (release) {
          const { id, version } = release.manifest.release;
          logger.info(`rendering with release ${id} (version ${version})`);
        } else {
          logger.warn(
            "no release configured (release, or edge + deliveryKey): pages render their inline defaults",
          );
        }
        const pub: PublicConfig = {
          edge: options.edge,
          deliveryKey: options.deliveryKey,
          environment,
          publicKeys: options.publicKeys,
          routing: resolveRouting(options, config, release),
          prerender: options.prerender ?? "static",
          inline: options.inline ?? "auto",
        };
        updateConfig({
          vite: {
            plugins: [
              virtualModules({
                "virtual:glossa/config": pub,
                "virtual:glossa/release": release ?? null,
              }),
            ],
            // The package imports virtual modules, so Vite has to process it, not pre-bundle or externalize it.
            optimizeDeps: { exclude: ["@glossa/astro"] },
            ssr: { noExternal: ["@glossa/astro"] },
          },
        });
        addMiddleware({ entrypoint: new URL("./middleware.js", import.meta.url), order: "pre" });
        if (options.elements) {
          const elements = fileURLToPath(new URL("./elements.js", import.meta.url));
          injectScript("page", `import ${JSON.stringify(elements)};`);
        }
      },
    },
  };
}
