import { mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { runInit } from "./commands/init.js";
import { runPull } from "./commands/pull.js";
import { runPush } from "./commands/push.js";
import { runScan } from "./commands/scan.js";
import { loadConfig, resolveApiKey, type GlossaConfig } from "./config.js";
import { EXIT_NETWORK, EXIT_OK, EXIT_PARTIAL } from "./exit.js";
import { scanSources, toScanInputs } from "./scan.js";

const here = dirname(fileURLToPath(import.meta.url));
const fixture = resolve(here, "__fixtures__", "project");

/** Returns a silent Console so tests don't spam stdout. */
function silentLog(): Console {
  return {
    ...console,
    log: vi.fn(),
    error: vi.fn(),
  } as unknown as Console;
}

describe("scan extraction", () => {
  it("finds keys across multiple glossa-* element types", async () => {
    const hits = await scanSources(fixture, ["src/**/*.tsx"]);
    const names = hits.map((h) => h.name);
    expect(names).toContain("cart.checkout");
    expect(names).toContain("athlete.greeting");
    expect(names).toContain("athlete.session_count");
    expect(names).toContain("user.gender");
  });

  it("returns hits sorted deterministically", async () => {
    const hits = await scanSources(fixture, ["src/**/*.tsx"]);
    const sorted = [...hits].sort(
      (a, b) => a.name.localeCompare(b.name) || a.file.localeCompare(b.file) || a.line - b.line,
    );
    expect(hits).toEqual(sorted);
  });

  it("honours .gitignore (skips src/generated/)", async () => {
    const hits = await scanSources(fixture, ["src/**/*.tsx"]);
    expect(hits.find((h) => h.name === "generated.should.skip")).toBeUndefined();
  });

  it("skips node_modules without a .gitignore entry for it", async () => {
    const hits = await scanSources(fixture, ["**/*.tsx"]);
    expect(hits.find((h) => h.name === "should.not.appear")).toBeUndefined();
  });

  it("dedupes by key name for scan inputs", async () => {
    const hits = await scanSources(fixture, ["src/**/*.tsx"]);
    const inputs = toScanInputs(hits);
    const names = inputs.map((i) => i.name);
    // cart.checkout appears in two files; should be reduced to one input.
    expect(names.filter((n) => n === "cart.checkout")).toHaveLength(1);
  });
});

describe("config", () => {
  it("resolveApiKey prefers config field over env", () => {
    const cfg: GlossaConfig = {
      project: "demo",
      apiUrl: "http://x",
      locales: ["de"],
      scan: ["src/**/*.ts"],
      outDir: "out",
      apiKey: "from-config",
    };
    expect(resolveApiKey(cfg, { GLOSSA_API_KEY: "from-env" })).toBe("from-config");
  });

  it("resolveApiKey falls through to env when config is null", () => {
    const cfg: GlossaConfig = {
      project: "demo",
      apiUrl: "http://x",
      locales: ["de"],
      scan: ["src/**/*.ts"],
      outDir: "out",
      apiKey: null,
    };
    expect(resolveApiKey(cfg, { GLOSSA_API_KEY: "from-env" })).toBe("from-env");
  });

  it("resolveApiKey throws when nothing is set", () => {
    const cfg: GlossaConfig = {
      project: "demo",
      apiUrl: "http://x",
      locales: ["de"],
      scan: ["src/**/*.ts"],
      outDir: "out",
      apiKey: null,
    };
    expect(() => resolveApiKey(cfg, {})).toThrow(/API key/);
  });
});

describe("glossa init", () => {
  let tmp: string;
  beforeEach(async () => {
    tmp = await mkdtemp(resolve(tmpdir(), "glossa-init-"));
  });
  afterEach(async () => {
    await rm(tmp, { recursive: true, force: true });
  });

  it("scaffolds glossa.config.json and is readable by loadConfig", async () => {
    const log = silentLog();
    const code = await runInit({ cwd: tmp, project: "demo", apiUrl: "http://x" }, log);
    expect(code).toBe(EXIT_OK);
    const cfg = await loadConfig(tmp);
    expect(cfg.project).toBe("demo");
    expect(cfg.apiUrl).toBe("http://x");
    expect(cfg.locales).toEqual(["de", "en"]);
  });

  it("refuses to overwrite without --force", async () => {
    const log = silentLog();
    await runInit({ cwd: tmp, project: "demo" }, log);
    const code = await runInit({ cwd: tmp, project: "demo" }, log);
    expect(code).not.toBe(EXIT_OK);
  });
});

describe("glossa scan command", () => {
  let tmp: string;
  beforeEach(async () => {
    tmp = await mkdtemp(resolve(tmpdir(), "glossa-scan-"));
    await writeFile(
      resolve(tmp, "glossa.config.json"),
      JSON.stringify({
        project: "demo",
        apiUrl: "http://x",
        locales: ["de"],
        scan: ["src/**/*.tsx"],
        outDir: "out",
        apiKey: "key",
      }),
    );
    // Plant a minimal source tree inside tmp so the scanner has
    // something to find without reaching outside its cwd.
    const { mkdir } = await import("node:fs/promises");
    await mkdir(resolve(tmp, "src"), { recursive: true });
    await writeFile(
      resolve(tmp, "src", "cart.tsx"),
      `export const C = <glossa-text key="cart.checkout">x</glossa-text>;\nexport const G = <glossa-rich key="athlete.greeting">hi</glossa-rich>;\n`,
    );
  });
  afterEach(async () => {
    await rm(tmp, { recursive: true, force: true });
  });

  it("returns EXIT_PARTIAL when the API reports per-row errors", async () => {
    const fetchMock = vi.fn(async () =>
      new Response(
        JSON.stringify({
          results: [
            { name: "cart.checkout", id: "u-1" },
            { name: "athlete.greeting", error: "boom" },
          ],
        }),
        { status: 200 },
      ),
    ) as unknown as typeof fetch;

    const code = await runScan({ cwd: tmp, fetch: fetchMock }, silentLog());
    expect(code).toBe(EXIT_PARTIAL);
  });

  it("returns EXIT_NETWORK on a 5xx", async () => {
    const fetchMock = vi.fn(
      async () => new Response("", { status: 503, statusText: "boom" }),
    ) as unknown as typeof fetch;
    const code = await runScan({ cwd: tmp, fetch: fetchMock }, silentLog());
    expect(code).toBe(EXIT_NETWORK);
  });
});

describe("glossa pull", () => {
  let tmp: string;
  beforeEach(async () => {
    tmp = await mkdtemp(resolve(tmpdir(), "glossa-pull-"));
    await writeFile(
      resolve(tmp, "glossa.config.json"),
      JSON.stringify({
        project: "demo",
        apiUrl: "http://x",
        locales: ["de", "en"],
        scan: ["src/**/*.tsx"],
        outDir: "out",
        apiKey: "key",
      }),
    );
  });
  afterEach(async () => {
    await rm(tmp, { recursive: true, force: true });
  });

  it("writes deterministic JSON with sorted keys", async () => {
    const bundles: Record<string, { project: string; locale: string; messages: Record<string, string>; statuses: Record<string, string> }> = {
      de: {
        project: "demo",
        locale: "de",
        messages: { zebra: "Z", alpha: "A", "cart.checkout": "Zur Kasse" },
        statuses: {},
      },
      en: {
        project: "demo",
        locale: "en",
        messages: { alpha: "A", zebra: "Z" },
        statuses: {},
      },
    };
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      const m = String(input).match(/\/locales\/([^/]+)\/messages$/);
      const locale = m?.[1] ?? "";
      return new Response(JSON.stringify(bundles[locale]), { status: 200 });
    }) as unknown as typeof fetch;

    const code = await runPull({ cwd: tmp, fetch: fetchMock }, silentLog());
    expect(code).toBe(EXIT_OK);

    const de = await readFile(resolve(tmp, "out", "de.json"), "utf8");
    const parsed = JSON.parse(de);
    expect(Object.keys(parsed.messages)).toEqual(["alpha", "cart.checkout", "zebra"]);
    // Same input twice should produce byte-identical output.
    await runPull({ cwd: tmp, fetch: fetchMock }, silentLog());
    const deAgain = await readFile(resolve(tmp, "out", "de.json"), "utf8");
    expect(deAgain).toBe(de);
  });
});

describe("glossa push", () => {
  let tmp: string;
  beforeEach(async () => {
    tmp = await mkdtemp(resolve(tmpdir(), "glossa-push-"));
    await writeFile(
      resolve(tmp, "glossa.config.json"),
      JSON.stringify({
        project: "demo",
        apiUrl: "http://x",
        locales: ["de"],
        scan: ["src/**/*.tsx"],
        outDir: "out",
        apiKey: "key",
      }),
    );
    await writeFile(
      resolve(tmp, "bundle.json"),
      JSON.stringify({ locale: "de", messages: { a: "A", b: "B" } }),
    );
  });
  afterEach(async () => {
    await rm(tmp, { recursive: true, force: true });
  });

  it("PATCHes one request per key and returns 0 on full success", async () => {
    const calls: string[] = [];
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      calls.push(`${init?.method ?? "GET"} ${String(input)}`);
      return new Response("", { status: 200 });
    }) as unknown as typeof fetch;
    const code = await runPush({ cwd: tmp, bundlePath: "bundle.json", fetch: fetchMock }, silentLog());
    expect(code).toBe(EXIT_OK);
    expect(calls).toHaveLength(2);
    expect(calls[0]).toContain("PATCH");
    expect(calls.some((c) => c.includes("/keys/a"))).toBe(true);
    expect(calls.some((c) => c.includes("/keys/b"))).toBe(true);
  });
});
