// A small static site built by test/build.test.ts with the integration from ../../dist.
import { fileURLToPath } from "node:url";
import vue from "@astrojs/vue";
import { defineConfig } from "astro/config";

import glossa from "../../dist/index.js";

const dist = fileURLToPath(new URL("../../dist/", import.meta.url));

export default defineConfig({
  i18n: { locales: ["de", "en"], defaultLocale: "de" },
  integrations: [
    vue({
      appEntrypoint: "@glossa/astro/vue",
      template: { compilerOptions: { isCustomElement: (tag) => tag.startsWith("glossa-") } },
    }),
    glossa({ release: "./.glossa-release", elements: true }),
  ],
  vite: {
    // What installing @glossa/astro from npm would resolve to.
    resolve: {
      alias: [{ find: /^@glossa\/astro\/(.+)$/, replacement: `${dist}$1.js` }],
      dedupe: ["vue"],
    },
  },
});
