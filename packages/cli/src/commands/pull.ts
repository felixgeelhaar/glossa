// `glossa pull` — fetch every locale's bundle, write to disk.
// Deterministic JSON output (sorted keys) so a `git diff` after
// `pull` is meaningful — a translator can review what shipped
// without noise from key-order shuffles.

import { mkdir, writeFile } from "node:fs/promises";
import { resolve } from "node:path";

import { createClient } from "@felixgeelhaar/glossa-sdk";

import { loadConfig, resolveApiKey } from "../config.js";
import { EXIT_NETWORK, EXIT_OK } from "../exit.js";

export interface PullOptions {
  cwd: string;
  fetch?: typeof fetch;
}

export async function runPull(opts: PullOptions, log: Console = console): Promise<number> {
  const cfg = await loadConfig(opts.cwd);
  const apiKey = resolveApiKey(cfg);

  const client = createClient({
    project: cfg.project,
    apiUrl: cfg.apiUrl,
    apiKey,
    ...(opts.fetch ? { fetch: opts.fetch } : {}),
  });

  const outDir = resolve(opts.cwd, cfg.outDir);
  await mkdir(outDir, { recursive: true });

  for (const locale of cfg.locales) {
    let bundle;
    try {
      bundle = await client.bundle(locale);
    } catch (err) {
      log.error(`pull ${locale}: ${(err as Error).message}`);
      return EXIT_NETWORK;
    }
    // Sort keys for deterministic diffs. Status map sorted on the
    // same key order so the two files line up visually.
    const sortedMessages: Record<string, string> = {};
    for (const k of Object.keys(bundle.messages).sort()) {
      sortedMessages[k] = bundle.messages[k] as string;
    }
    const out = JSON.stringify(
      { project: bundle.project, locale: bundle.locale, messages: sortedMessages },
      null,
      2,
    );
    const path = resolve(outDir, `${locale}.json`);
    await writeFile(path, out + "\n", "utf8");
    log.log(`pull ${locale}: ${Object.keys(sortedMessages).length} keys → ${path}`);
  }
  return EXIT_OK;
}
