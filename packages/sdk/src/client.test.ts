import { describe, expect, it, vi } from "vitest";

import { createClient, GlossaError } from "./client.js";
import type { Bundle, ScanResponse } from "./types.js";

const baseConfig = {
  project: "demo",
  apiKey: "glossa_abc",
  apiUrl: "https://glossa.example.com",
};

const bundleA: Bundle = {
  project: "demo",
  locale: "de",
  messages: { "cart.checkout": "Zur Kasse" },
  statuses: { "cart.checkout": "approved" },
};

/**
 * makeFetch builds a vitest-mock `fetch` that the SDK accepts via
 * its `fetch` config option. Each call resolves with the queued
 * response in order; passing a function lets the script inspect
 * the request (headers, body) before responding.
 */
function makeFetch(...responses: Array<Response | ((req: Request) => Response | Promise<Response>)>) {
  const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const next = responses.shift();
    if (!next) throw new Error("makeFetch: no more queued responses");
    if (typeof next === "function") {
      return next(new Request(input as URL, init));
    }
    return next;
  });
  return fetchMock as unknown as typeof fetch;
}

describe("createClient", () => {
  it("requires project, apiKey, apiUrl", () => {
    expect(() => createClient({ ...baseConfig, project: "" })).toThrow(/project/);
    expect(() => createClient({ ...baseConfig, apiKey: "" })).toThrow(/apiKey/);
    expect(() => createClient({ ...baseConfig, apiUrl: "" })).toThrow(/apiUrl/);
  });
});

describe("client.bundle", () => {
  it("returns the parsed bundle on a 200", async () => {
    const fetchMock = makeFetch(
      new Response(JSON.stringify(bundleA), {
        status: 200,
        headers: { "Content-Type": "application/json", ETag: "v1" },
      }),
    );
    const client = createClient({ ...baseConfig, fetch: fetchMock });

    const out = await client.bundle("de");
    expect(out).toEqual(bundleA);
  });

  it("sends Bearer auth + Accept and hits the right URL", async () => {
    let seen!: Request;
    const fetchMock = makeFetch(async (req) => {
      seen = req;
      return new Response(JSON.stringify(bundleA), { status: 200 });
    });

    const client = createClient({ ...baseConfig, fetch: fetchMock });
    await client.bundle("de");

    expect(seen.url).toBe("https://glossa.example.com/api/v1/projects/demo/locales/de/messages");
    expect(seen.headers.get("Authorization")).toBe("Bearer glossa_abc");
    expect(seen.headers.get("Accept")).toBe("application/json");
  });

  it("sends If-None-Match on a second call and returns cached bundle on 304", async () => {
    const fetchMock = makeFetch(
      new Response(JSON.stringify(bundleA), {
        status: 200,
        headers: { ETag: "v1" },
      }),
      async (req) => {
        expect(req.headers.get("If-None-Match")).toBe("v1");
        return new Response(null, { status: 304 });
      },
    );

    const client = createClient({ ...baseConfig, fetch: fetchMock });
    const first = await client.bundle("de");
    const second = await client.bundle("de");
    expect(second).toEqual(first);
  });

  it("throws a GlossaError carrying the HTTP status on a 401", async () => {
    const fetchMock = makeFetch(new Response("", { status: 401, statusText: "Unauthorized" }));
    const client = createClient({ ...baseConfig, fetch: fetchMock });

    try {
      await client.bundle("de");
      throw new Error("expected throw");
    } catch (err) {
      expect(err).toBeInstanceOf(GlossaError);
      expect((err as GlossaError).status).toBe(401);
    }
  });
});

describe("client.message", () => {
  it("returns the cached message for a known key", async () => {
    const fetchMock = makeFetch(new Response(JSON.stringify(bundleA), { status: 200 }));
    const client = createClient({ ...baseConfig, fetch: fetchMock });
    await client.bundle("de");
    expect(client.message("de", "cart.checkout")).toBe("Zur Kasse");
  });

  it("returns undefined for a missing key (acceptance criterion)", async () => {
    const fetchMock = makeFetch(new Response(JSON.stringify(bundleA), { status: 200 }));
    const client = createClient({ ...baseConfig, fetch: fetchMock });
    await client.bundle("de");
    expect(client.message("de", "no.such.key")).toBeUndefined();
  });

  it("returns undefined before any bundle has been fetched", () => {
    const client = createClient({ ...baseConfig, fetch: makeFetch() });
    expect(client.message("de", "anything")).toBeUndefined();
  });
});

describe("client.scan", () => {
  it("POSTs the keys array and parses the response", async () => {
    const expected: ScanResponse = {
      results: [{ name: "cart.checkout", id: "00000000-0000-0000-0000-000000000001" }],
    };
    let seenBody: string | undefined;
    const fetchMock = makeFetch(async (req) => {
      seenBody = await req.text();
      return new Response(JSON.stringify(expected), { status: 200 });
    });
    const client = createClient({ ...baseConfig, fetch: fetchMock });

    const out = await client.scan([{ name: "cart.checkout" }]);
    expect(out).toEqual(expected);
    expect(JSON.parse(seenBody ?? "{}")).toEqual({ keys: [{ name: "cart.checkout" }] });
  });
});
