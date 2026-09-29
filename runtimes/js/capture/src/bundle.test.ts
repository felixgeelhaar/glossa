// @vitest-environment node
/**
 * Capture code never reaches a production bundle (RFC 0004 §5.1): an app
 * built on the runtime and every component package contains none of it, and
 * the capture package itself tree-shakes. And what does ship stays inside the
 * package's size budget (RFC 0005 §5.1).
 */
import { execFileSync } from "node:child_process";
import { createRequire } from "node:module";
import { dirname, join } from "node:path";
import { describe, expect, it } from "vitest";
import { build } from "esbuild";

async function bundle(contents: string): Promise<string> {
  const out = await build({
    stdin: { contents, resolveDir: import.meta.dirname, loader: "ts" },
    bundle: true,
    minify: true,
    format: "esm",
    write: false,
    platform: "browser",
    external: ["vue", "react", "react-dom", "lit", "@lit/context"],
    logLevel: "silent",
  });
  return out.outputFiles[0]!.text;
}

/** Things only capture code has: the start mark, range geometry, the session API. */
const CAPTURE = [/\u2063|\\u2063/, /getClientRects/, /startCapture|collectRegions|stripMarkers/];
/** Things only the visual probe pass has (RFC 0005 \u00a75.2). */
const PROBES = /glossa\.finding\/v1|untranslated-on-screen|text-clipped/;

describe("tree-shaking", () => {
  it("an app on @glossa/runtime and the components has no capture code", async () => {
    const js = await bundle(`
      import { createRuntime } from "@glossa/runtime";
      import { GlossaText } from "@glossa/elements";
      import { createGlossa as vue, GlossaText as VueText } from "@glossa/vue";
      import { createGlossa as react, T } from "@glossa/react";
      const rt = createRuntime({ locales: "de" });
      console.log(rt.t("x"), GlossaText, vue({ runtime: rt }), VueText, react({ runtime: rt }), T);
    `);
    expect(js).toContain("onRender"); // the runtime's extension point is there…
    for (const re of CAPTURE) expect(js).not.toMatch(re); // …the capture module isn't.
  });

  it("the capture package tree-shakes: markers alone don't pull in the capture script", async () => {
    const js = await bundle(`import { strip } from "@glossa/capture"; console.log(strip("x"));`);
    expect(js).not.toMatch(/getClientRects|onRender/);
    const all = await bundle(`import { startCapture } from "@glossa/capture"; console.log(startCapture);`);
    expect(all).toMatch(/getClientRects/);
  });

  /**
   * The probe pass is given to a session, never imported by it, so a bundle
   * that measures nothing — `@glossa/overlay`, the in-product editor served to
   * end users — doesn't carry it (RFC 0005 §5.1).
   */
  it("a session without the probe pass leaves ./probes out; asking for it brings it in", async () => {
    const session = await bundle(`
      import { startCapture } from "@glossa/capture";
      console.log(startCapture([]).collect());
    `);
    expect(session).toMatch(/getClientRects/); // the capture script is there…
    expect(session).not.toMatch(PROBES); // …the probes aren't.

    const probing = await bundle(`
      import { startCapture } from "@glossa/capture";
      import { probe } from "@glossa/capture/probes";
      console.log(startCapture([], { probe }).collect());
    `);
    expect(probing).toMatch(PROBES);
  });
});

/**
 * Both budgets, from the one `size-limit` block of package.json: RFC 0005
 * §5.1's 4 kB for a session **with** the probe pass, which is what `glossa
 * capture` runs, and the unchanged 3 kB for a session **without** it, which is
 * what `@glossa/overlay` pulls in. The measurement is `size-limit`, the same
 * tool every runtime package's `pnpm size` runs — one place to change a
 * budget, and no second mechanism to disagree with it. Run as a test because
 * `pnpm -r test` is what CI runs for these packages; it needs `dist/`, exactly
 * as the tree-shaking tests above do.
 */
describe("the size budgets", () => {
  it("a session fits 3 kB brotli, and 4 kB with the probe pass", () => {
    const pkg = join(import.meta.dirname, "..");
    const bin = join(dirname(createRequire(import.meta.url).resolve("size-limit/package.json")), "bin.js");
    // Non-zero exit (over budget, or no dist/) throws with size-limit's own report.
    const report = execFileSync(process.execPath, [bin], { cwd: pkg, encoding: "utf8" });
    expect(report).toContain("Size limit: 4 kB");
    expect(report).toContain("Size limit: 3 kB");
    expect(report).not.toMatch(/exceeded/);
    expect(report.match(/Size:\s+[\d.]+ kB with all dependencies, minified and brotlied/g)).toHaveLength(2);
  }, 120_000);
});
