// @vitest-environment node
/**
 * Capture code never reaches a production bundle (RFC 0004 §5.1): an app
 * built on the runtime and every component package contains none of it, and
 * the capture package itself tree-shakes.
 */
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
});
