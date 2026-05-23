// `glossa push <bundle.json>` — push a translated bundle back.
// Used by translators who export, edit offline, then re-import.
//
// Wire: per-key PATCH against
//   /api/v1/projects/:slug/locales/:locale/keys/:key
// using the SDK's exported HTTP transport. Per-row errors are
// surfaced; the command exits EXIT_PARTIAL if any row fails.

import { readFile } from "node:fs/promises";
import { resolve } from "node:path";

import { loadConfig, resolveApiKey } from "../config.js";
import { EXIT_CONFIG, EXIT_NETWORK, EXIT_OK, EXIT_PARTIAL } from "../exit.js";

interface PushFile {
  project?: string;
  locale: string;
  messages: Record<string, string>;
}

export interface PushOptions {
  cwd: string;
  bundlePath: string;
  fetch?: typeof fetch;
}

export async function runPush(opts: PushOptions, log: Console = console): Promise<number> {
  const cfg = await loadConfig(opts.cwd);
  const apiKey = resolveApiKey(cfg);

  let parsed: PushFile;
  try {
    const raw = await readFile(resolve(opts.cwd, opts.bundlePath), "utf8");
    parsed = JSON.parse(raw) as PushFile;
  } catch (err) {
    log.error(`push: read ${opts.bundlePath}: ${(err as Error).message}`);
    return EXIT_CONFIG;
  }
  if (!parsed.locale || typeof parsed.messages !== "object") {
    log.error(`push: ${opts.bundlePath} missing "locale" or "messages"`);
    return EXIT_CONFIG;
  }
  if (parsed.project && parsed.project !== cfg.project) {
    log.error(`push: bundle project "${parsed.project}" != config "${cfg.project}"`);
    return EXIT_CONFIG;
  }

  const doFetch = opts.fetch ?? fetch;
  const base = `${cfg.apiUrl.replace(/\/+$/, "")}/api/v1/projects/${encodeURIComponent(cfg.project)}/locales/${encodeURIComponent(parsed.locale)}/keys`;

  let failed = 0;
  let ok = 0;
  for (const [key, value] of Object.entries(parsed.messages)) {
    const res = await doFetch(`${base}/${encodeURIComponent(key)}`, {
      method: "PATCH",
      headers: {
        Authorization: "Bearer " + apiKey,
        "Content-Type": "application/json",
      },
      body: JSON.stringify({ value }),
    });
    if (!res.ok) {
      const detail = await safeText(res);
      log.error(`push: ${key}: ${res.status} ${res.statusText} ${detail}`);
      failed++;
    } else {
      ok++;
    }
  }
  log.log(`push: ${ok} updated, ${failed} failed`);
  if (failed > 0) return failed === ok + failed ? EXIT_NETWORK : EXIT_PARTIAL;
  return EXIT_OK;
}

async function safeText(res: Response): Promise<string> {
  try {
    return (await res.text()).slice(0, 200);
  } catch {
    return "";
  }
}
