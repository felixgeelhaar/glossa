/**
 * Builds test/fixture with `astro build` (static output, Vue islands,
 * i18n routing) and checks what ships: translated HTML from the build-time
 * release, islands rendered on the server with the page's runtime, and the
 * inline release slice they hydrate from. Needs `pnpm build` first (the
 * fixture uses ../../dist, like an installed package).
 */
import { execFile } from "node:child_process";
import { mkdir, readFile, readdir, rm, writeFile } from "node:fs/promises";
import { createRequire } from "node:module";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { promisify } from "node:util";
import { beforeAll, describe, expect, it } from "vitest";

import { INLINE_ID } from "../src/page.js";
import type { InlineRelease } from "../src/page.js";
import { match, msg, release, text } from "../src/testing/release.js";

const fixture = fileURLToPath(new URL("./fixture/", import.meta.url));
const astro = join(
  dirname(createRequire(import.meta.url).resolve("astro/package.json")),
  "astro.js",
);

const r = release("rel_42", 42, {
  de: {
    "home.title": text("Willkommen"),
    "cart.items": match("count", "number", {
      one: ["ein Artikel"],
      "*": [{ $: "count" }, " Artikel"],
    }),
    greeting: msg("Hallo, ", { $: "name" }, "!"),
    terms: msg("Lies die ", { open: "b" }, "AGB", { close: "b" }, "."),
  },
  en: {
    "home.title": text("Welcome"),
    "cart.items": match("count", "number", { one: ["one item"], "*": [{ $: "count" }, " items"] }),
    greeting: msg("Hello, ", { $: "name" }, "!"),
    terms: msg("Read the ", { open: "b" }, "terms", { close: "b" }, "."),
  },
});

const page = (path: string) => readFile(join(fixture, "dist", path), "utf8");

const inlined = (html: string): InlineRelease | undefined => {
  const m = new RegExp(`<script type="application/json" id="${INLINE_ID}">(.*?)</script>`).exec(
    html,
  );
  return m ? (JSON.parse(m[1]!) as InlineRelease) : undefined;
};

beforeAll(async () => {
  const dir = join(fixture, ".glossa-release");
  await rm(dir, { recursive: true, force: true });
  await mkdir(join(dir, "a"), { recursive: true });
  await writeFile(join(dir, "manifest.json"), JSON.stringify(r.manifest));
  for (const [sha, bytes] of Object.entries(r.bytes)) {
    await writeFile(join(dir, "a", `${sha}.json`), bytes);
  }
  await promisify(execFile)(process.execPath, [astro, "build", "--root", fixture], {
    env: { ...process.env, ASTRO_TELEMETRY_DISABLED: "1" },
  });
}, 60_000);

describe("astro build with glossa()", () => {
  it("renders .astro pages and <glossa-*> elements with the release, per locale", async () => {
    const de = await page("index.html");
    expect(de).toContain('<html lang="de" dir="ltr"');
    expect(de).toContain("<title>Willkommen</title>");
    expect(de).toMatch(/<glossa-provider lang="de" dir="ltr"/);
    expect(de).toMatch(
      /<glossa-text key="home.title" data-allow-mismatch="">Willkommen<\/glossa-text>/,
    );
    expect(de).toMatch(/<glossa-plural key="cart.items" count="3"[^>]*>3 Artikel</);
    expect(de).toMatch(/<glossa-text key="only.inline">Nur inline<\/glossa-text>/);
    const en = await page("en/index.html");
    expect(en).toContain('<html lang="en" dir="ltr"');
    expect(en).toContain("<title>Welcome</title>");
    expect(en).toMatch(/>Welcome<\/glossa-text>/);
    expect(en).toContain('<link rel="alternate" hreflang="de" href="/">');
    expect(en).toContain('<link rel="alternate" hreflang="en" href="/en/">');
  });

  it("renders Vue islands on the server with the page's locale", async () => {
    const de = await page("index.html");
    expect(de).toContain('<section class="island" lang="de">');
    expect(de).toContain("Hallo, ⁨Lina⁩!");
    expect(de).toContain("Lies die <b>AGB</b>.");
    const en = await page("en/index.html");
    expect(en).toContain("Hello, ⁨Lina⁩!");
    expect(en).toContain("Read the <b>terms</b>.");
    // Vue drops `key`, so elements inside .vue files use message=.
    expect(en).toContain(
      '<p class="e"><glossa-text message="home.title" data-allow-mismatch="">Welcome</glossa-text>',
    );
  });

  it("inlines the page locale's release slice for hydration, and only where something hydrates", async () => {
    const slice = inlined(await page("en/index.html"))!;
    expect(slice.locale).toBe("en");
    expect(slice.release.manifest.release.id).toBe("rel_42");
    expect(Object.keys(slice.release.artifacts).sort()).toEqual(
      [r.manifest.artifacts.en!.default!.sha256, r.manifest.artifacts.de!.default!.sha256].sort(),
    );
    const plain = await page("plain/index.html");
    expect(plain).toContain('<p id="t">Willkommen</p>');
    expect(plain).toContain(">Willkommen</glossa-text>");
    expect(inlined(plain)).toBeUndefined();
  });

  it("keeps the release out of client JavaScript", async () => {
    const assets = join(fixture, "dist", "_astro");
    const js = (await readdir(assets)).filter((f) => f.endsWith(".js"));
    expect(js.length).toBeGreaterThan(0);
    for (const f of js) {
      const code = await readFile(join(assets, f), "utf8");
      expect(code).not.toContain("Willkommen");
      expect(code).not.toContain("Welcome");
    }
  });
});
