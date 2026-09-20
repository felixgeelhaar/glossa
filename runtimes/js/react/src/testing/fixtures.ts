/**
 * The runtime contract's conformance fixtures (runtimes/testdata), test-only:
 * the same shapes @glossa/runtime's contract tests read.
 */
import { readdirSync, readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import type { Manifest } from "@glossa/runtime";

import type { EdgeManifest } from "./edge.js";

// A string, not `new URL(…)`: under jsdom, `URL` is jsdom's, which node:url rejects.
const testdata = join(dirname(fileURLToPath(import.meta.url)), "../../../../testdata");

export interface ScenarioCase {
  requested: string[];
  id: string;
  values: Record<string, unknown>;
  default?: string;
  bidiIsolation: "none" | "default";
  exp: string;
  expLocale: string;
  expChain: string[];
  expResolvedFrom: string | null;
  expDirection?: "ltr" | "rtl";
}

export interface Scenario {
  description: string;
  manifest: Manifest;
  artifacts: Record<string, string>;
  cases: ScenarioCase[];
}

export interface LoadingStep {
  description: string;
  edge: { manifest: EdgeManifest; artifacts: Record<string, string> };
  read: { id: string; requested: string[] };
  expActiveRelease: string | null;
  expSource: string;
  expErrors: string[];
  exp: string;
}

export interface LoadingSequence {
  description: string;
  publicKeys: Array<{ keyId: string; key: string }>;
  steps: LoadingStep[];
  restartBefore?: number[];
}

/** Every `*.json` in a fixture directory, by file name without extension. */
export function fixtures<T>(dir: "scenarios" | "loading"): Array<[string, T]> {
  const path = join(testdata, dir);
  return readdirSync(path)
    .filter((f) => f.endsWith(".json"))
    .sort()
    .map((f) => [f.slice(0, -5), JSON.parse(readFileSync(join(path, f), "utf8")) as T]);
}
