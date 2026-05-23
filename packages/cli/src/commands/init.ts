// `glossa init` — scaffold a default glossa.config.json. Idempotent
// in the failure case: refuses to overwrite an existing file so a
// repeated `glossa init` can't clobber a hand-edited config.

import { stat } from "node:fs/promises";
import { resolve } from "node:path";

import { writeDefaultConfig, type GlossaConfig } from "../config.js";
import { EXIT_CONFIG, EXIT_OK } from "../exit.js";

export interface InitOptions {
  cwd: string;
  /** Override project slug. Defaults to the cwd basename. */
  project?: string;
  /** Locales to seed. Defaults to ["de", "en"]. */
  locales?: string[];
  apiUrl?: string;
  force?: boolean;
}

export async function runInit(opts: InitOptions, log: Console = console): Promise<number> {
  const path = resolve(opts.cwd, "glossa.config.json");
  if (!opts.force && (await exists(path))) {
    log.error(`glossa.config.json already exists at ${path} (use --force to overwrite)`);
    return EXIT_CONFIG;
  }

  const project = opts.project ?? defaultProject(opts.cwd);
  const cfg: GlossaConfig = {
    project,
    apiUrl: opts.apiUrl ?? "http://localhost:8080",
    locales: opts.locales ?? ["de", "en"],
    scan: ["src/**/*.{ts,tsx,js,jsx,html}"],
    outDir: "glossa-bundles",
    apiKey: null,
  };

  const written = await writeDefaultConfig(opts.cwd, cfg);
  log.log(`wrote ${written}`);
  log.log("next: set GLOSSA_API_KEY then run `glossa scan`");
  return EXIT_OK;
}

async function exists(path: string): Promise<boolean> {
  try {
    await stat(path);
    return true;
  } catch (err) {
    if ((err as NodeJS.ErrnoException).code === "ENOENT") return false;
    throw err;
  }
}

function defaultProject(cwd: string): string {
  const last = cwd.split(/[\\/]/).filter(Boolean).pop() ?? "project";
  // Project slugs are lowercase + dashes per the OpenAPI pattern.
  return last.toLowerCase().replace(/[^a-z0-9-]+/g, "-").replace(/^-+|-+$/g, "") || "project";
}
