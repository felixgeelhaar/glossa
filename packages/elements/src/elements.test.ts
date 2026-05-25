import { afterEach, describe, expect, it, vi } from "vitest";

import type { Bundle } from "@felixgeelhaar/glossa-sdk";

import "./index.js";
import type { GlossaProvider } from "./glossa-provider.js";

const de: Bundle = {
  project: "demo",
  locale: "de",
  messages: {
    "cart.checkout": "Zur Kasse",
    "athlete.greeting": "Hallo, {name}!",
    "athlete.session_count": "{count, plural, =0 {keine Einheiten} one {eine Einheit} other {# Einheiten}}",
    "user.gender": "{value, select, female {Sie} male {Er} other {Sie}}",
  },
  statuses: { "cart.checkout": "approved" },
};

const en: Bundle = {
  project: "demo",
  locale: "en",
  messages: { "cart.checkout": "Checkout" },
  statuses: {},
};

/**
 * makeFetch responds to bundle requests with the matching seed
 * and stubs out /sse with a stream that never emits — keeps each
 * test deterministic without dealing with timers.
 */
function makeFetch(bundles: Record<string, Bundle>): typeof fetch {
  return (async (input: RequestInfo | URL) => {
    const url = String(input);
    if (url.endsWith("/sse")) {
      return new Response(new ReadableStream(), {
        status: 200,
        headers: { "Content-Type": "text/event-stream" },
      });
    }
    const m = url.match(/\/locales\/([^/]+)\/messages$/);
    if (m?.[1] && bundles[m[1]]) {
      return new Response(JSON.stringify(bundles[m[1]]), { status: 200 });
    }
    return new Response("", { status: 404 });
  }) as typeof fetch;
}

async function mountProvider(html: string, fetchImpl: typeof fetch): Promise<GlossaProvider> {
  const container = document.createElement("div");
  container.innerHTML = html;
  document.body.appendChild(container);
  const provider = container.querySelector("glossa-provider") as GlossaProvider;
  // Inject the test fetch BEFORE attribute-driven boot kicks off.
  provider.fetchImpl = fetchImpl;
  provider.connectedCallback();
  await flush();
  return provider;
}

async function flush(): Promise<void> {
  for (let i = 0; i < 10; i++) await Promise.resolve();
}

/**
 * renderedText returns the shadow-DOM text of `el` minus any
 * <style> tag content. JSDOM concatenates inline stylesheet text
 * into shadowRoot.textContent because it doesn't implement
 * adoptedStyleSheets — Lit falls back to <style> tags and the
 * style text leaks into a naive .textContent read.
 */
function renderedText(el: Element): string {
  const root = el.shadowRoot;
  if (!root) return "";
  let text = "";
  for (const node of Array.from(root.childNodes)) {
    if (node.nodeType === Node.COMMENT_NODE) continue; // lit-html part markers
    if ((node as Element).tagName === "STYLE") continue;
    text += node.textContent ?? "";
  }
  return text.trim();
}

afterEach(() => {
  document.body.innerHTML = "";
});

describe("<glossa-provider> + <glossa-text>", () => {
  it("renders the translated value once the bundle loads", async () => {
    const fetchImpl = makeFetch({ de });
    const provider = await mountProvider(
      `<glossa-provider project="demo" locale="de" api-url="https://glossa.test" api-key="glossa_x">
         <glossa-text key="cart.checkout">Approve</glossa-text>
       </glossa-provider>`,
      fetchImpl,
    );
    await provider.updateComplete;
    await flush();
    const text = provider.querySelector("glossa-text")!;
    await text.shadowRoot!.querySelector("slot"); // ensure shadow root exists
    expect(renderedText(text)).toBe("Zur Kasse");
  });

  it("falls back to slot content when the key is missing", async () => {
    const fetchImpl = makeFetch({ de });
    const provider = await mountProvider(
      `<glossa-provider project="demo" locale="de" api-url="https://glossa.test" api-key="glossa_x">
         <glossa-text key="no.such.key">Default Label</glossa-text>
       </glossa-provider>`,
      fetchImpl,
    );
    await provider.updateComplete;
    await flush();
    const text = provider.querySelector("glossa-text")!;
    expect(text.textContent?.trim()).toBe("Default Label");
  });

  it("switches every visible string when locale changes", async () => {
    const fetchImpl = makeFetch({ de, en });
    const provider = await mountProvider(
      `<glossa-provider project="demo" locale="de" api-url="https://glossa.test" api-key="glossa_x">
         <glossa-text key="cart.checkout">…</glossa-text>
       </glossa-provider>`,
      fetchImpl,
    );
    await provider.updateComplete;
    await flush();
    const text = provider.querySelector("glossa-text")!;
    expect(renderedText(text)).toBe("Zur Kasse");

    provider.locale = "en";
    await provider.updateComplete;
    await flush();
    expect(renderedText(text)).toBe("Checkout");
  });

  it("warns on missing key when strict mode is on", async () => {
    const warn = vi.spyOn(console, "warn").mockImplementation(() => {});
    const fetchImpl = makeFetch({ de });
    const provider = await mountProvider(
      `<glossa-provider project="demo" locale="de" api-url="https://glossa.test" api-key="glossa_x" strict>
         <glossa-text key="no.such.key">Fallback</glossa-text>
       </glossa-provider>`,
      fetchImpl,
    );
    await provider.updateComplete;
    await flush();
    expect(warn).toHaveBeenCalledWith(expect.stringContaining("no.such.key"));
    warn.mockRestore();
  });
});

describe("<glossa-rich>", () => {
  it("interpolates vars via @felixgeelhaar/glossa-format", async () => {
    const fetchImpl = makeFetch({ de });
    const provider = await mountProvider(
      `<glossa-provider project="demo" locale="de" api-url="https://glossa.test" api-key="glossa_x">
         <glossa-rich key="athlete.greeting" vars='{"name":"Sophia"}'>Hi</glossa-rich>
       </glossa-provider>`,
      fetchImpl,
    );
    await provider.updateComplete;
    await flush();
    const rich = provider.querySelector("glossa-rich")!;
    expect(renderedText(rich)).toBe("Hallo, Sophia!");
  });
});

describe("<glossa-plural>", () => {
  it("selects the right plural arm by count and locale", async () => {
    const fetchImpl = makeFetch({ de });
    const provider = await mountProvider(
      `<glossa-provider project="demo" locale="de" api-url="https://glossa.test" api-key="glossa_x">
         <glossa-plural key="athlete.session_count" count="3">no sessions</glossa-plural>
       </glossa-provider>`,
      fetchImpl,
    );
    await provider.updateComplete;
    await flush();
    const plural = provider.querySelector("glossa-plural")!;
    expect(renderedText(plural)).toBe("3 Einheiten");
  });
});

describe("<glossa-select>", () => {
  it("picks the matching arm by value", async () => {
    const fetchImpl = makeFetch({ de });
    const provider = await mountProvider(
      `<glossa-provider project="demo" locale="de" api-url="https://glossa.test" api-key="glossa_x">
         <glossa-select key="user.gender" value="male">they</glossa-select>
       </glossa-provider>`,
      fetchImpl,
    );
    await provider.updateComplete;
    await flush();
    const sel = provider.querySelector("glossa-select")!;
    expect(renderedText(sel)).toBe("Er");
  });
});

describe("<glossa-selector>", () => {
  it("emits glossa-locale-change with source=manual on user pick", async () => {
    const fetchImpl = makeFetch({ de, en });
    const provider = await mountProvider(
      `<glossa-provider project="demo" locale="de" api-url="https://glossa.test" api-key="glossa_x">
         <glossa-selector locales="en,de" labels="English,Deutsch"></glossa-selector>
       </glossa-provider>`,
      fetchImpl,
    );
    await provider.updateComplete;
    await flush();

    const selector = provider.querySelector("glossa-selector")!;
    await (selector as HTMLElement & { updateComplete: Promise<unknown> }).updateComplete;
    const select = selector.shadowRoot!.querySelector("select")!;

    const events: CustomEvent[] = [];
    selector.addEventListener("glossa-locale-change", (e) => events.push(e as CustomEvent));

    select.value = "en";
    select.dispatchEvent(new Event("change"));

    expect(events).toHaveLength(1);
    expect(events[0].detail).toEqual({ locale: "en", source: "manual" });
  });

  it("emits source=auto on first connect when browser language differs", async () => {
    const fetchImpl = makeFetch({ de, en });
    const provider = await mountProvider(
      `<glossa-provider project="demo" locale="de" api-url="https://glossa.test" api-key="glossa_x">
         <glossa-selector locales="en,de" auto-detect></glossa-selector>
       </glossa-provider>`,
      fetchImpl,
    );
    await provider.updateComplete;
    await flush();

    const selector = provider.querySelector("glossa-selector") as HTMLElement & {
      detectImpl?: () => string | undefined;
    };
    // Reset autoDetected so we can drive the path deterministically.
    (selector as unknown as { autoDetected: boolean }).autoDetected = false;
    selector.detectImpl = () => "en-GB";

    const events: CustomEvent[] = [];
    selector.addEventListener("glossa-locale-change", (e) => events.push(e as CustomEvent));

    // Re-trigger the detect path by reattaching the element.
    (selector as unknown as { maybeAutoDetect: () => void }).maybeAutoDetect();

    expect(events).toHaveLength(1);
    expect(events[0].detail).toEqual({ locale: "en", source: "auto" });
  });

  it("does NOT emit auto when browser language matches current locale", async () => {
    const fetchImpl = makeFetch({ de, en });
    const provider = await mountProvider(
      `<glossa-provider project="demo" locale="de" api-url="https://glossa.test" api-key="glossa_x">
         <glossa-selector locales="en,de" auto-detect></glossa-selector>
       </glossa-provider>`,
      fetchImpl,
    );
    await provider.updateComplete;
    await flush();

    const selector = provider.querySelector("glossa-selector") as HTMLElement & {
      detectImpl?: () => string | undefined;
    };
    (selector as unknown as { autoDetected: boolean }).autoDetected = false;
    selector.detectImpl = () => "de-DE";

    const events: CustomEvent[] = [];
    selector.addEventListener("glossa-locale-change", (e) => events.push(e as CustomEvent));

    (selector as unknown as { maybeAutoDetect: () => void }).maybeAutoDetect();
    expect(events).toHaveLength(0);
  });

  it("does NOT emit auto when browser language isn't in the locales list", async () => {
    const fetchImpl = makeFetch({ de, en });
    const provider = await mountProvider(
      `<glossa-provider project="demo" locale="de" api-url="https://glossa.test" api-key="glossa_x">
         <glossa-selector locales="en,de" auto-detect></glossa-selector>
       </glossa-provider>`,
      fetchImpl,
    );
    await provider.updateComplete;
    await flush();

    const selector = provider.querySelector("glossa-selector") as HTMLElement & {
      detectImpl?: () => string | undefined;
    };
    (selector as unknown as { autoDetected: boolean }).autoDetected = false;
    selector.detectImpl = () => "fr-FR";

    const events: CustomEvent[] = [];
    selector.addEventListener("glossa-locale-change", (e) => events.push(e as CustomEvent));

    (selector as unknown as { maybeAutoDetect: () => void }).maybeAutoDetect();
    expect(events).toHaveLength(0);
  });

  it("renders the current locale read-only when locales attribute is missing", async () => {
    const fetchImpl = makeFetch({ de });
    const provider = await mountProvider(
      `<glossa-provider project="demo" locale="de" api-url="https://glossa.test" api-key="glossa_x">
         <glossa-selector label="Sprache"></glossa-selector>
       </glossa-provider>`,
      fetchImpl,
    );
    await provider.updateComplete;
    await flush();

    const selector = provider.querySelector("glossa-selector")!;
    expect(renderedText(selector)).toBe("de");
    // No <select> element — read-only mode.
    expect(selector.shadowRoot!.querySelector("select")).toBeNull();
  });
});
