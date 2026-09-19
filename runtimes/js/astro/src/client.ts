/**
 * `@glossa/astro/client`: the page's one runtime, shared by every island and
 * `<glossa-provider>` on it. It starts from the release slice the page
 * inlined (so the first render matches the server's HTML) and then refreshes
 * from the edge like any browser runtime. On the server, during an island's
 * render, it's the request's runtime instead.
 */
import { createRuntime } from "@glossa/runtime";
import type { Runtime } from "@glossa/runtime";
import config from "virtual:glossa/config";

import type { InlineRelease } from "./page.js";

let page: Runtime | undefined;

/** The runtime for this page (browser) or this request (server). */
export function getRuntime(): Runtime {
  const server = (globalThis as Record<symbol, (() => Runtime) | undefined>)[
    Symbol.for("glossa.astro.server")
  ];
  if (server) return server();
  if (page) return page;
  let inline: Partial<InlineRelease> = {};
  try {
    inline = JSON.parse(document.getElementById("glossa-release")?.textContent ?? "{}");
  } catch {
    // No inline release: the runtime starts from persisted storage or the edge.
  }
  return (page = createRuntime({
    edge: config.edge,
    deliveryKey: config.deliveryKey,
    environment: config.environment,
    publicKeys: config.publicKeys,
    locales: inline.locale ?? (document.documentElement.lang || undefined),
    bundled: inline.release,
  }));
}
