// Library exports — lets the tests (and any future programmatic
// integration) drive the commands without going through the bin
// shim. The bin entrypoint lives in bin.ts.

export { ConfigError, loadConfig, resolveApiKey, writeDefaultConfig } from "./config.js";
export type { GlossaConfig } from "./config.js";
export { scanSources, toScanInputs } from "./scan.js";
export type { KeyHit } from "./scan.js";
export { runInit } from "./commands/init.js";
export { runScan } from "./commands/scan.js";
export { runPull } from "./commands/pull.js";
export { runPush } from "./commands/push.js";
export * from "./exit.js";
