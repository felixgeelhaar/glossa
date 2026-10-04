/** `@felixgeelhaar/glossa-unplugin` for Rollup: `import glossa from "@felixgeelhaar/glossa-unplugin/rollup"`. List it after the plugins that compile TS, JSX or SFCs. */
import { glossa } from "./plugin.js";

export type { GlossaPluginOptions } from "./options.js";
export default glossa.rollup;
