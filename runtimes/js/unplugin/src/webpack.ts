/** `@felixgeelhaar/glossa-unplugin` for webpack: `import glossa from "@felixgeelhaar/glossa-unplugin/webpack"`. It runs as a post loader. */
import { glossa } from "./plugin.js";

export type { GlossaPluginOptions } from "./options.js";
export default glossa.webpack;
