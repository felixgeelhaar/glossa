// glossa.config.json loader. The format is intentionally tiny:
// project / apiUrl / locales / scan globs / outDir. JSON only for
// the MVP — adding .ts support means dragging in tsx or jiti, and
// the entire config is data, no logic.

import { readFile, writeFile } from "node:fs/promises";
import { resolve } from "node:path";

export interface GlossaConfig {
  /** Project slug, must match what the API was bootstrapped with. */
  project: string;
  /** API base URL, no trailing slash. */
  apiUrl: string;
  /** BCP-47 locale codes managed by this repo. */
  locales: string[];
  /**
   * Glob patterns relative to repo root. `scan` walks source files
   * matching these patterns to extract glossa-* element keys.
   */
  scan: string[];
  /** Directory `glossa pull` writes per-locale bundles to. */
  outDir: string;
  /**
   * API key for build-time calls. Read from `GLOSSA_API_KEY` env
   * by default — `null` here means the loader will require the
   * env var at use time. Allowing literal values in the file is a
   * convenience for one-off scripts, never for committed configs.
   */
  apiKey?: string | null;
}

export const DEFAULT_CONFIG_FILE = "glossa.config.json";

export class ConfigError extends Error {
  public constructor(message: string) {
    super(message);
    this.name = "ConfigError";
  }
}

/**
 * Load glossa.config.json from the given cwd. Throws a
 * [[ConfigError]] (caller maps to EXIT_CONFIG) on any failure —
 * missing file, malformed JSON, missing required fields.
 */
export async function loadConfig(cwd: string, file = DEFAULT_CONFIG_FILE): Promise<GlossaConfig> {
  const path = resolve(cwd, file);
  let raw: string;
  try {
    raw = await readFile(path, "utf8");
  } catch (err) {
    if ((err as NodeJS.ErrnoException).code === "ENOENT") {
      throw new ConfigError(`config not found: ${path} (run \`glossa init\` first)`);
    }
    throw new ConfigError(`read ${path}: ${(err as Error).message}`);
  }
  let parsed: unknown;
  try {
    parsed = JSON.parse(raw);
  } catch (err) {
    throw new ConfigError(`parse ${path}: ${(err as Error).message}`);
  }
  return validate(parsed, path);
}

function validate(parsed: unknown, path: string): GlossaConfig {
  if (typeof parsed !== "object" || parsed === null) {
    throw new ConfigError(`${path}: expected an object`);
  }
  const v = parsed as Record<string, unknown>;
  const required = ["project", "apiUrl", "locales", "scan", "outDir"] as const;
  for (const k of required) {
    if (v[k] === undefined) {
      throw new ConfigError(`${path}: missing required field "${k}"`);
    }
  }
  if (typeof v["project"] !== "string") throw new ConfigError(`${path}: "project" must be a string`);
  if (typeof v["apiUrl"] !== "string") throw new ConfigError(`${path}: "apiUrl" must be a string`);
  if (typeof v["outDir"] !== "string") throw new ConfigError(`${path}: "outDir" must be a string`);
  if (!Array.isArray(v["locales"]) || v["locales"].some((l) => typeof l !== "string")) {
    throw new ConfigError(`${path}: "locales" must be an array of strings`);
  }
  if (!Array.isArray(v["scan"]) || v["scan"].some((g) => typeof g !== "string")) {
    throw new ConfigError(`${path}: "scan" must be an array of glob strings`);
  }
  return {
    project: v["project"] as string,
    apiUrl: v["apiUrl"] as string,
    locales: v["locales"] as string[],
    scan: v["scan"] as string[],
    outDir: v["outDir"] as string,
    apiKey: typeof v["apiKey"] === "string" ? (v["apiKey"] as string) : null,
  };
}

/**
 * Resolve the API key to use for HTTP calls. Order: explicit
 * config field → GLOSSA_API_KEY env var. Throws [[ConfigError]]
 * if neither is set so the CLI fails fast rather than producing
 * 401s.
 */
export function resolveApiKey(cfg: GlossaConfig, env: NodeJS.ProcessEnv = process.env): string {
  const fromConfig = cfg.apiKey;
  if (typeof fromConfig === "string" && fromConfig !== "") return fromConfig;
  const fromEnv = env["GLOSSA_API_KEY"];
  if (typeof fromEnv === "string" && fromEnv !== "") return fromEnv;
  throw new ConfigError("no API key — set GLOSSA_API_KEY or apiKey in glossa.config.json");
}

/** Write a default config to disk; used by `glossa init`. */
export async function writeDefaultConfig(cwd: string, cfg: GlossaConfig): Promise<string> {
  const path = resolve(cwd, DEFAULT_CONFIG_FILE);
  await writeFile(path, JSON.stringify(cfg, null, 2) + "\n", "utf8");
  return path;
}
