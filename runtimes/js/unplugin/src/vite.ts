/** `@glossa/unplugin` for Vite (and Astro, through `@glossa/astro`): `import glossa from "@glossa/unplugin/vite"`. */
import { glossa } from "./plugin.js";

export type { GlossaPluginOptions, StudioOptions } from "./options.js";
export default glossa.vite;
