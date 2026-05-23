import { afterEach, describe, expect, it } from "vitest";

import type { Bundle, TranslationStatus } from "@glossa/sdk";

import "@glossa/elements";

import "./admin-app.js";
import "./demo-strip.js";
import "./key-edit.js";
import "./key-list.js";
import type { GlossaAdmin } from "./admin-app.js";
import type { GlossaAdminKeyEdit } from "./key-edit.js";
import type { GlossaAdminKeyList } from "./key-list.js";

const bundle: Bundle = {
  project: "demo",
  locale: "de",
  messages: {
    "cart.checkout": "Zur Kasse",
    "athlete.session_count": "{count, plural, one {Eine Einheit} other {# Einheiten}}",
  },
  statuses: {
    "cart.checkout": "approved",
    "athlete.session_count": "needs_review",
  },
};

interface PatchCapture {
  url: string;
  body: { value: string; status: TranslationStatus };
}

function makeFetch(opts: { capture?: PatchCapture[] } = {}): typeof fetch {
  return (async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input);
    const method = init?.method ?? "GET";
    if (url.endsWith("/sse")) {
      return new Response(new ReadableStream(), {
        status: 200,
        headers: { "Content-Type": "text/event-stream" },
      });
    }
    if (method === "GET" && url.endsWith("/messages")) {
      return new Response(JSON.stringify(bundle), { status: 200 });
    }
    if (method === "PATCH") {
      const body = init?.body ? JSON.parse(String(init.body)) : null;
      opts.capture?.push({ url, body: body as PatchCapture["body"] });
      return new Response("", { status: 200 });
    }
    return new Response("", { status: 404 });
  }) as typeof fetch;
}

async function mountAdmin(fetchImpl: typeof fetch): Promise<GlossaAdmin> {
  localStorage.setItem(
    "glossa-admin-settings-v1",
    JSON.stringify({ apiUrl: "https://glossa.test", apiKey: "glossa_x", project: "demo", locale: "de" }),
  );
  const el = document.createElement("glossa-admin") as GlossaAdmin;
  el.fetchImpl = fetchImpl;
  document.body.appendChild(el);
  // Wait for the bundle fetch to land AND the child key-list to
  // render rows. The parent's render sets the messages prop on
  // the child synchronously, but the child schedules its own
  // update in a microtask; we need to give that microtask a chance
  // to run after the parent commits.
  for (let i = 0; i < 80; i++) {
    await new Promise((r) => setTimeout(r, 0));
    await el.updateComplete;
    const list = el.shadowRoot?.querySelector("glossa-admin-key-list") as
      | (HTMLElement & { updateComplete: Promise<boolean> })
      | null;
    if (list) {
      // Two cycles: first commits the prop change from the parent's
      // render, second waits for the child's induced update.
      await list.updateComplete;
      await new Promise((r) => setTimeout(r, 0));
      await list.updateComplete;
      if (list.shadowRoot?.querySelector("tbody tr")) return el;
    }
  }
  return el;
}

afterEach(() => {
  for (const child of Array.from(document.body.children)) {
    document.body.removeChild(child);
  }
  localStorage.clear();
});

describe("<glossa-admin> golden path", () => {
  it("renders the bundle and runs key-edit → PATCH end to end (acceptance criterion)", async () => {
    const capture: PatchCapture[] = [];
    const admin = await mountAdmin(makeFetch({ capture }));

    const list = admin.shadowRoot!.querySelector("glossa-admin-key-list") as GlossaAdminKeyList;
    expect(list).toBeTruthy();
    await list.updateComplete;
    const rows = list.shadowRoot!.querySelectorAll("tbody tr");
    expect(rows.length).toBe(2);

    // a11y: rows are keyboard-focusable. Asserts the table is a
    // proper grid before we drive interactions through it.
    expect(rows[0]!.getAttribute("tabindex")).toBe("0");

    // Click the first row → editor mounts.
    (rows[0] as HTMLElement).click();
    await admin.updateComplete;
    const editor = admin.shadowRoot!.querySelector("glossa-admin-key-edit") as GlossaAdminKeyEdit;
    expect(editor).toBeTruthy();
    await editor.updateComplete;

    // Drop in a new value, hit Save.
    const textarea = editor.shadowRoot!.querySelector("textarea") as HTMLTextAreaElement;
    textarea.value = "Jetzt kaufen";
    textarea.dispatchEvent(new Event("input"));
    await editor.updateComplete;

    const form = editor.shadowRoot!.querySelector("form") as HTMLFormElement;
    form.dispatchEvent(new Event("submit", { cancelable: true, bubbles: true }));
    // Give the async save handler a chance to fire.
    for (let i = 0; i < 30; i++) await Promise.resolve();

    expect(capture.length).toBe(1);
    expect(capture[0]!.body.value).toBe("Jetzt kaufen");
    expect(capture[0]!.body.status).toBe("needs_review");
    expect(capture[0]!.url).toContain("/locales/de/keys/");
  });
});

describe("<glossa-admin-key-edit>", () => {
  it("renders a live ICU preview against the draft value", async () => {
    const el = document.createElement("glossa-admin-key-edit") as GlossaAdminKeyEdit;
    el.keyName = "athlete.session_count";
    el.locale = "de";
    el.value = "{count, plural, one {Eine Einheit} other {# Einheiten}}";
    document.body.appendChild(el);
    await el.updateComplete;
    for (let i = 0; i < 10; i++) await Promise.resolve();

    const preview = el.shadowRoot!.querySelector(".preview") as HTMLElement;
    expect(preview.textContent).toContain("2 Einheiten");
  });
});

describe("<glossa-admin-key-list>", () => {
  it("filters by status when the filter prop is set", async () => {
    const el = document.createElement("glossa-admin-key-list") as GlossaAdminKeyList;
    el.messages = bundle.messages;
    el.statuses = bundle.statuses;
    el.filter = "approved";
    document.body.appendChild(el);
    await el.updateComplete;
    const rows = el.shadowRoot!.querySelectorAll("tbody tr");
    expect(rows.length).toBe(1);
    expect(rows[0]!.textContent).toContain("cart.checkout");
  });
});
