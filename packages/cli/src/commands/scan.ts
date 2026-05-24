// `glossa scan` — walk source, extract keys, POST batch upsert.
//
// Exit codes:
//   0  every row succeeded (or there were no rows)
//   2  request itself failed (network / 5xx / auth)
//   3  request succeeded but the API reported per-row errors

import { createClient } from "@felixgeelhaar/glossa-sdk";

import { loadConfig, resolveApiKey } from "../config.js";
import { EXIT_NETWORK, EXIT_OK, EXIT_PARTIAL } from "../exit.js";
import { scanSources, toScanInputs } from "../scan.js";

export interface ScanOptions {
  cwd: string;
  /** Test seam — vitest swaps in a mock fetch. */
  fetch?: typeof fetch;
}

export async function runScan(opts: ScanOptions, log: Console = console): Promise<number> {
  const cfg = await loadConfig(opts.cwd);
  const apiKey = resolveApiKey(cfg);

  const hits = await scanSources(opts.cwd, cfg.scan);
  const inputs = toScanInputs(hits);

  if (inputs.length === 0) {
    log.log("scan: 0 keys found — nothing to push");
    return EXIT_OK;
  }
  log.log(`scan: ${inputs.length} unique keys across ${hits.length} occurrences`);

  const client = createClient({
    project: cfg.project,
    apiUrl: cfg.apiUrl,
    apiKey,
    ...(opts.fetch ? { fetch: opts.fetch } : {}),
  });

  let response;
  try {
    response = await client.scan(inputs);
  } catch (err) {
    log.error(`scan: API error: ${(err as Error).message}`);
    return EXIT_NETWORK;
  }

  const failures = response.results.filter((r) => r.error !== undefined);
  if (failures.length > 0) {
    for (const f of failures) {
      log.error(`scan: ${f.name}: ${f.error}`);
    }
    log.error(`scan: ${failures.length}/${response.results.length} keys failed`);
    return EXIT_PARTIAL;
  }
  log.log(`scan: ${response.results.length}/${inputs.length} keys upserted`);
  return EXIT_OK;
}
