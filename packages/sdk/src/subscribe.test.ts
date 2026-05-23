import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { createClient } from "./client.js";
import type { Bundle, TranslationUpdatedEvent } from "./types.js";

const baseConfig = {
  project: "demo",
  apiKey: "glossa_abc",
  apiUrl: "https://glossa.example.com",
};

const initialBundle: Bundle = {
  project: "demo",
  locale: "de",
  messages: { "cart.checkout": "Zur Kasse" },
  statuses: { "cart.checkout": "approved" },
};

/**
 * makeSSE returns a Response whose body is a ReadableStream the
 * caller can write SSE frames into. The chunks helper writes one
 * frame per call (id + event + json data + blank line).
 */
function makeSSE(opts: { onClose?: () => void } = {}) {
  let controller!: ReadableStreamDefaultController<Uint8Array>;
  const stream = new ReadableStream<Uint8Array>({
    start(c) {
      controller = c;
    },
    cancel() {
      opts.onClose?.();
    },
  });
  const encoder = new TextEncoder();
  return {
    response: new Response(stream, {
      status: 200,
      headers: { "Content-Type": "text/event-stream" },
    }),
    push(event: TranslationUpdatedEvent, id: number): void {
      const frame = `id: ${id}\nevent: translation.updated\ndata: ${JSON.stringify(event)}\n\n`;
      controller.enqueue(encoder.encode(frame));
    },
    closeStream(): void {
      controller.close();
    },
  };
}

describe("subscribe", () => {
  beforeEach(() => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
  });
  afterEach(() => {
    vi.useRealTimers();
  });

  it("delivers translation.updated events to the handler", async () => {
    const sse = makeSSE();
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url.endsWith("/messages")) {
        return new Response(JSON.stringify(initialBundle), { status: 200 });
      }
      return sse.response;
    }) as unknown as typeof fetch;

    const client = createClient({ ...baseConfig, fetch: fetchMock });
    await client.bundle("de");

    const received: TranslationUpdatedEvent[] = [];
    const sub = client.subscribe({
      onEvent: (e) => received.push(e),
    });

    // Wait a tick so the loop has opened the request.
    await Promise.resolve();
    await Promise.resolve();

    sse.push(
      {
        type: "translation.updated",
        project: "demo",
        locale: "de",
        key: "cart.checkout",
        value: "Jetzt kaufen",
        status: "approved",
      },
      42,
    );

    // Give the parser one microtask cycle.
    for (let i = 0; i < 10; i++) await Promise.resolve();

    expect(received).toHaveLength(1);
    expect(received[0]?.value).toBe("Jetzt kaufen");
    // Cache patched without a re-fetch.
    expect(client.message("de", "cart.checkout")).toBe("Jetzt kaufen");

    sub.close();
  });

  it("auto-reconnects after a failed connection (acceptance criterion)", async () => {
    // First fetch: rejects. Second fetch: succeeds with an SSE
    // stream. Verifies the backoff loop kicks in and retries.
    let calls = 0;
    let opened = 0;
    const sse = makeSSE();
    const fetchMock = vi.fn(async () => {
      calls++;
      if (calls === 1) throw new Error("ECONNREFUSED");
      return sse.response;
    }) as unknown as typeof fetch;

    const client = createClient({ ...baseConfig, fetch: fetchMock });

    const sub = client.subscribe({
      onOpen: () => {
        opened++;
      },
      onError: () => {
        // swallow — backoff loop logs and retries
      },
    });

    // Initial failure happens immediately; advance through the
    // 500ms backoff to let the retry fire.
    for (let i = 0; i < 5; i++) await Promise.resolve();
    await vi.advanceTimersByTimeAsync(600);
    for (let i = 0; i < 10; i++) await Promise.resolve();

    expect(calls).toBeGreaterThanOrEqual(2);
    expect(opened).toBe(1);

    sub.close();
  });

  it("close() aborts the in-flight fetch signal", async () => {
    let capturedSignal: AbortSignal | undefined;
    const sse = makeSSE();
    const fetchMock = vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
      capturedSignal = init?.signal ?? undefined;
      return sse.response;
    }) as unknown as typeof fetch;

    const client = createClient({ ...baseConfig, fetch: fetchMock });
    const sub = client.subscribe({});
    for (let i = 0; i < 5; i++) await Promise.resolve();

    expect(capturedSignal?.aborted).toBe(false);
    sub.close();
    expect(capturedSignal?.aborted).toBe(true);
  });
});
