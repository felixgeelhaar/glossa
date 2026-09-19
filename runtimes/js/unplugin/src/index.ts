/**
 * `@glossa/unplugin`: finds where every message is used, in any bundler.
 *
 * ```ts
 * // vite.config.ts
 * import glossa from "@glossa/unplugin/vite";
 *
 * export default defineConfig({
 *   plugins: [vue(), glossa({ application: "web", routes: { "/checkout": ["src/checkout/**"] } })],
 * });
 * ```
 *
 * The build writes `.glossa/usages.json` (`glossa.usages/v1`) beside its
 * output; `glossa context push .glossa/usages.json` uploads it.
 */
export { glossa, glossa as default, unpluginFactory, PLUGIN_NAME } from "./plugin.js";
export type { GlossaPluginOptions } from "./options.js";
export type { Routes, Usage, UsageKind, UsagesDocument } from "./usage.js";
export { accessorPaths } from "./keys.js";
