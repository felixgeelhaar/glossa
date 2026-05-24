#!/usr/bin/env node
// Bin entrypoint. Hand-rolled argv parsing — yargs/commander are
// 40-100kb on disk and we need exactly four commands with no flag
// complexity worth importing them for.

import { runInit } from "./commands/init.js";
import { runPull } from "./commands/pull.js";
import { runPush } from "./commands/push.js";
import { runScan } from "./commands/scan.js";
import { ConfigError } from "./config.js";
import { EXIT_CONFIG } from "./exit.js";

async function main(): Promise<number> {
  const [, , cmd, ...rest] = process.argv;
  const cwd = process.cwd();

  try {
    switch (cmd) {
      case "init":
        return runInit({
          cwd,
          force: rest.includes("--force"),
          yes: rest.includes("--yes") || rest.includes("-y"),
        });
      case "scan":
        return runScan({ cwd });
      case "pull":
        return runPull({ cwd });
      case "push": {
        const file = rest[0];
        if (!file) {
          console.error("usage: glossa push <bundle.json>");
          return EXIT_CONFIG;
        }
        return runPush({ cwd, bundlePath: file });
      }
      case undefined:
      case "-h":
      case "--help":
        printHelp();
        return 0;
      default:
        console.error(`unknown command: ${cmd}`);
        printHelp();
        return EXIT_CONFIG;
    }
  } catch (err) {
    if (err instanceof ConfigError) {
      console.error(`glossa: ${err.message}`);
      return EXIT_CONFIG;
    }
    console.error("glossa:", (err as Error).message);
    return 1;
  }
}

function printHelp(): void {
  console.log(`glossa — translation pipeline CLI

Usage:
  glossa init [--yes] [--force]  interactive scaffold of glossa.config.json
                                 (--yes accepts every default for CI use)
  glossa scan                  walk source, push discovered keys
  glossa pull                  fetch all locale bundles to outDir
  glossa push <bundle.json>    push a translated bundle back

Environment:
  GLOSSA_API_KEY               Bearer key used for API calls`);
}

main()
  .then((code) => process.exit(code))
  .catch((err) => {
    console.error("glossa: fatal:", err);
    process.exit(1);
  });
