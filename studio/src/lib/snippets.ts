/**
 * Ready-to-paste integration code for a delivery key, one per runtime
 * (runtimes/SPEC.md §2: runtimes fetch
 * `{edge}/v1/{deliveryKey}/{environment}/manifest.json`). With signing
 * keys, the snippets pin them so unsigned manifests are rejected (§1.3).
 */

export interface SnippetInput {
  /** glossa-edge origin, no trailing slash. */
  edge: string;
  deliveryKey: string;
  environment: string;
  /** Active signing keys: key id and base64url public key. */
  publicKeys: ReadonlyArray<{ keyId: string; key: string }>;
}

export type SnippetId = "runtime" | "vue" | "elements" | "go";

export interface Snippet {
  id: SnippetId;
  label: string;
  /** Package or module to install. */
  install: string;
  language: "ts" | "html" | "go";
  code: string;
}

/** Placeholder when neither the runtime configuration nor the build names glossa-edge. */
export const EDGE_PLACEHOLDER = "https://edge.example.com";

/** An absolute http(s) origin without a trailing slash, or undefined. */
function origin(v: unknown): string | undefined {
  if (typeof v !== "string" || !v.trim()) return undefined;
  try {
    const u = new URL(v.trim());
    return u.protocol === "https:" || u.protocol === "http:" ? u.href.replace(/\/+$/, "") : undefined;
  } catch {
    return undefined;
  }
}

/** The edge origin Studio was built with (VITE_GLOSSA_EDGE_URL). */
export function configuredEdge(env: Record<string, unknown> = import.meta.env): string | undefined {
  return origin(env.VITE_GLOSSA_EDGE_URL);
}

let runtimeEdge: Promise<string | undefined> | undefined;

/**
 * glossa-edge's origin: `edgeUrl` from the runtime configuration the
 * Studio image serves at /config.json (GLOSSA_STUDIO_EDGE_URL), else the
 * build's VITE_GLOSSA_EDGE_URL. Undefined when neither names one (dev
 * servers answer /config.json with the SPA, which isn't JSON).
 */
export function edgeOrigin(fetchConfig: () => Promise<Response> = () => globalThis.fetch("/config.json", { cache: "no-store" })): Promise<string | undefined> {
  runtimeEdge ??= fetchConfig()
    .then(async (r) => (r.ok ? origin(((await r.json()) as { edgeUrl?: unknown }).edgeUrl) : undefined))
    .catch(() => undefined)
    .then((edge) => edge ?? configuredEdge());
  return runtimeEdge;
}

/** Tests only: forget the cached origin. */
export function resetEdgeOrigin(): void {
  runtimeEdge = undefined;
}

const q = JSON.stringify;

function jsKeys(keys: SnippetInput["publicKeys"], indent: string): string {
  if (!keys.length) return "";
  const list = keys.map((k) => `{ keyId: ${q(k.keyId)}, key: ${q(k.key)} }`).join(", ");
  return `\n${indent}publicKeys: [${list}],`;
}

function runtime(i: SnippetInput): string {
  return `import { createRuntime } from "@glossa/runtime";

const glossa = createRuntime({
  edge: ${q(i.edge)},
  deliveryKey: ${q(i.deliveryKey)}, // publishable, read-only
  environment: ${q(i.environment)},${jsKeys(i.publicKeys, "  ")}
});

await glossa.ready;
console.log(glossa.t("app.title", {}, { default: "My app" }));`;
}

function vue(i: SnippetInput): string {
  return `import { createApp } from "vue";
import { createGlossa } from "@glossa/vue";
import App from "./App.vue";

createApp(App)
  .use(
    createGlossa({
      edge: ${q(i.edge)},
      deliveryKey: ${q(i.deliveryKey)},
      environment: ${q(i.environment)},${jsKeys(i.publicKeys, "      ")}
    }),
  )
  .mount("#app");`;
}

const attr = (s: string) => s.replace(/&/g, "&amp;").replace(/"/g, "&quot;");

function elements(i: SnippetInput): string {
  const keys = i.publicKeys.length ? `\n  public-keys="${attr(i.publicKeys.map((k) => `${k.keyId}:${k.key}`).join(","))}"` : "";
  return `<script type="module">
  import "@glossa/elements";
</script>

<glossa-provider
  edge="${attr(i.edge)}"
  delivery-key="${attr(i.deliveryKey)}"
  environment="${attr(i.environment)}"${keys}
>
  <glossa-text key="app.title">My app</glossa-text>
</glossa-provider>`;
}

function go(i: SnippetInput): string {
  const parse = i.publicKeys
    .map((k, n) => `\tkey${n + 1}, err := glossa.ParsePublicKey(${q(k.keyId)}, ${q(k.key)})\n\tif err != nil {\n\t\tlog.Fatal(err)\n\t}\n`)
    .join("");
  const keys = i.publicKeys.length ? `\n\t\tPublicKeys:  []glossa.PublicKey{${i.publicKeys.map((_, n) => `key${n + 1}`).join(", ")}},` : "";
  return `package main

import (
\t"fmt"
\t"log"

\tglossa "github.com/felixgeelhaar/glossa/runtimes/go"
)

func main() {
${parse}\tclient, err := glossa.New(glossa.Config{
\t\tEdgeURL:     ${q(i.edge)},
\t\tDeliveryKey: ${q(i.deliveryKey)},
\t\tEnvironment: ${q(i.environment)},${keys}
\t})
\tif err != nil {
\t\tlog.Fatal(err)
\t}
\tdefer client.Close()

\tfmt.Println(client.For("en").T("app.title", nil, glossa.Default("My app")))
}`;
}

export function snippets(i: SnippetInput): Snippet[] {
  return [
    { id: "runtime", label: "JavaScript", install: "npm install @glossa/runtime", language: "ts", code: runtime(i) },
    { id: "vue", label: "Vue", install: "npm install @glossa/vue", language: "ts", code: vue(i) },
    { id: "elements", label: "Web components", install: "npm install @glossa/elements", language: "html", code: elements(i) },
    { id: "go", label: "Go", install: "go get github.com/felixgeelhaar/glossa/runtimes/go", language: "go", code: go(i) },
  ];
}

/** Enough of a key to recognise it in a list. */
export const maskKey = (key: string): string => `${key.slice(0, "glossa_pk_".length + 4)}…`;
