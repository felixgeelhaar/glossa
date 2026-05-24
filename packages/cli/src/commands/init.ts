// `glossa init` — scaffold a glossa.config.json.
//
// Interactive by default: prompts for API URL, API key, project
// slug, and source locales. Non-interactive (--yes) accepts every
// default + reads overrides from flags / env so CI scripts can call
// it without a TTY.
//
// Idempotent in the failure case: refuses to overwrite an existing
// config so a repeated `glossa init` can't clobber a hand-edited
// file. Pass --force to override.

import { stat } from "node:fs/promises";
import { resolve } from "node:path";
import { createInterface } from "node:readline/promises";
import { stdin as input, stdout as output } from "node:process";

import { writeDefaultConfig, type GlossaConfig } from "../config.js";
import { EXIT_CONFIG, EXIT_OK } from "../exit.js";

export interface InitOptions {
  cwd: string;
  /** Override project slug. Defaults to the cwd basename. */
  project?: string;
  /** Locales to seed. Defaults to ["de", "en"]. */
  locales?: string[];
  apiUrl?: string;
  apiKey?: string;
  /** Skip prompts; accept every default. */
  yes?: boolean;
  /** Overwrite an existing config. */
  force?: boolean;
  /** Test seam: substitute readline.question for non-TTY tests. */
  ask?: (question: string, defaultAnswer?: string) => Promise<string>;
}

const DEFAULT_API_URL = "https://glossa.example.com";

export async function runInit(opts: InitOptions, log: Console = console): Promise<number> {
  const path = resolve(opts.cwd, "glossa.config.json");
  if (!opts.force && (await exists(path))) {
    log.error(`glossa.config.json already exists at ${path} (use --force to overwrite)`);
    return EXIT_CONFIG;
  }

  // Defaults — flag > env > derived fallback.
  const fallbackProject = defaultProject(opts.cwd);
  const fallbackApiUrl = opts.apiUrl ?? process.env.GLOSSA_API_URL ?? DEFAULT_API_URL;
  const fallbackApiKey = opts.apiKey ?? process.env.GLOSSA_API_KEY ?? "";
  const fallbackLocales = (opts.locales ?? ["de", "en"]).join(",");
  const fallbackSlug = opts.project ?? fallbackProject;

  let apiUrl = fallbackApiUrl;
  let apiKey = fallbackApiKey;
  let slug = fallbackSlug;
  let locales = opts.locales ?? ["de", "en"];

  // Skip prompts when stdin isn't a TTY (CI, piped input) unless the
  // caller explicitly supplied an `ask` seam. Avoids hanging tests
  // that call runInit programmatically without a terminal.
  const interactive = !opts.yes && (opts.ask !== undefined || input.isTTY);

  if (interactive) {
    const ask = opts.ask ?? makeAsk();
    try {
      apiUrl = (await ask("Glossa API URL", fallbackApiUrl)) || fallbackApiUrl;
      slug = (await ask("Project slug", fallbackSlug)) || fallbackSlug;
      const localesIn = (await ask("Locales (comma-separated)", fallbackLocales)) || fallbackLocales;
      locales = localesIn.split(",").map((s) => s.trim()).filter(Boolean);
      apiKey = (await ask("API key (paste from admin → API keys, blank = set via GLOSSA_API_KEY later)", fallbackApiKey)) || fallbackApiKey;
    } finally {
      if (!opts.ask) closeAsk();
    }
  }

  const cfg: GlossaConfig = {
    project: slug,
    apiUrl,
    locales,
    scan: ["src/**/*.{ts,tsx,js,jsx,html,vue,astro}"],
    outDir: "glossa-bundles",
    apiKey: apiKey || null,
  };

  const written = await writeDefaultConfig(opts.cwd, cfg);
  log.log(`wrote ${written}`);
  if (!apiKey) {
    log.log("next: set GLOSSA_API_KEY then run `glossa scan`");
  } else {
    log.log("next: run `glossa scan`");
  }
  return EXIT_OK;
}

// ── readline plumbing ──────────────────────────────────────────────

let rl: ReturnType<typeof createInterface> | undefined;

function makeAsk(): (question: string, defaultAnswer?: string) => Promise<string> {
  rl = createInterface({ input, output });
  return async (question, defaultAnswer) => {
    const prompt = defaultAnswer ? `${question} [${defaultAnswer}]: ` : `${question}: `;
    const answer = await rl!.question(prompt);
    return answer.trim() || (defaultAnswer ?? "");
  };
}

function closeAsk(): void {
  rl?.close();
  rl = undefined;
}

// ── helpers ────────────────────────────────────────────────────────

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
  return last.toLowerCase().replace(/[^a-z0-9-]+/g, "-").replace(/^-+|-+$/g, "") || "project";
}
