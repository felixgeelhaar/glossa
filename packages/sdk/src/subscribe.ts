// Fetch-based SSE reader with auto-reconnect.
//
// We don't use the browser EventSource because it can't send
// custom headers — the API key has to ride in `Authorization`, so
// every transport is a `fetch` request whose body we parse line
// by line. Same code path runs in Node (≥18 ships native fetch
// and ReadableStream) and in modern browsers.
//
// Reconnect: exponential backoff capped at 30s. The `Last-Event-ID`
// header is set to the most recent event ID seen so the server can
// replay anything missed from its history ring.

import type { TranslationUpdatedEvent } from "./types.js";

const INITIAL_BACKOFF_MS = 500;
const MAX_BACKOFF_MS = 30_000;
const BACKOFF_FACTOR = 2;

/** Subscriber options passed by callers. */
export interface SubscribeOptions {
  onEvent?: (e: TranslationUpdatedEvent) => void;
  /**
   * Fires whenever the stream errors (network blip, 5xx,
   * disconnect). The SDK will reconnect anyway; this is purely
   * informational for callers that want to surface "offline"
   * state in their UI.
   */
  onError?: (err: unknown) => void;
  /** Fires once each time the SSE handshake succeeds. */
  onOpen?: () => void;
  /** Abort the subscription externally. */
  signal?: AbortSignal;
}

/** Internal options shared with [[subscribe]]. */
export interface InternalSubscribeOptions extends SubscribeOptions {
  url: string;
  apiKey: string;
  fetch: typeof fetch;
}

/** Handle returned by [[subscribe]] — call .close() to tear down. */
export interface Subscription {
  close(): void;
}

/** Open an SSE connection with auto-reconnect. */
export function subscribe(opts: InternalSubscribeOptions): Subscription {
  const ctl = new AbortController();
  if (opts.signal) {
    if (opts.signal.aborted) ctl.abort();
    else opts.signal.addEventListener("abort", () => ctl.abort(), { once: true });
  }

  let lastEventID = "";
  let backoff = INITIAL_BACKOFF_MS;
  let closed = false;

  void runLoop();

  return {
    close(): void {
      closed = true;
      ctl.abort();
    },
  };

  async function runLoop(): Promise<void> {
    while (!closed && !ctl.signal.aborted) {
      try {
        await openOnce();
        // openOnce returns when the server gracefully ends the
        // stream — reconnect with the same backoff progression so
        // we don't hammer the server in a tight loop.
      } catch (err) {
        if (closed || ctl.signal.aborted) return;
        opts.onError?.(err);
      }
      if (closed || ctl.signal.aborted) return;
      await sleep(backoff, ctl.signal);
      backoff = Math.min(backoff * BACKOFF_FACTOR, MAX_BACKOFF_MS);
    }
  }

  async function openOnce(): Promise<void> {
    const headers = new Headers({
      Accept: "text/event-stream",
      Authorization: "Bearer " + opts.apiKey,
    });
    if (lastEventID) headers.set("Last-Event-ID", lastEventID);

    const res = await opts.fetch(opts.url, {
      method: "GET",
      headers,
      signal: ctl.signal,
    });
    if (!res.ok || !res.body) {
      throw new Error(`sse: ${res.status} ${res.statusText}`);
    }
    // Reset backoff after a successful handshake so a stable
    // connection doesn't carry forward an inflated delay.
    backoff = INITIAL_BACKOFF_MS;
    opts.onOpen?.();

    const reader = res.body.getReader();
    const decoder = new TextDecoder("utf-8");
    let buffer = "";

    while (!closed) {
      const { value, done } = await reader.read();
      if (done) return;
      buffer += decoder.decode(value, { stream: true });

      let sep = buffer.indexOf("\n\n");
      while (sep !== -1) {
        const block = buffer.slice(0, sep);
        buffer = buffer.slice(sep + 2);
        const frame = parseFrame(block);
        if (frame) {
          if (frame.id) lastEventID = frame.id;
          if (frame.event === "translation.updated" && frame.data) {
            try {
              const payload = JSON.parse(frame.data) as TranslationUpdatedEvent;
              opts.onEvent?.(payload);
            } catch (err) {
              opts.onError?.(err);
            }
          }
        }
        sep = buffer.indexOf("\n\n");
      }
    }
  }
}

interface ParsedFrame {
  id?: string;
  event?: string;
  data?: string;
}

/** Parse one SSE frame (newline-separated `key: value` lines). */
function parseFrame(block: string): ParsedFrame | null {
  if (!block) return null;
  const frame: ParsedFrame = {};
  for (const raw of block.split("\n")) {
    if (!raw || raw.startsWith(":")) continue;
    const colon = raw.indexOf(":");
    if (colon === -1) continue;
    const key = raw.slice(0, colon);
    const value = raw.slice(colon + 1).replace(/^ /, "");
    if (key === "id") frame.id = value;
    else if (key === "event") frame.event = value;
    else if (key === "data") frame.data = (frame.data ? frame.data + "\n" : "") + value;
  }
  return frame;
}

/** Promise-based sleep that resolves early on AbortSignal. */
function sleep(ms: number, signal: AbortSignal): Promise<void> {
  return new Promise<void>((resolve) => {
    const t = setTimeout(resolve, ms);
    signal.addEventListener(
      "abort",
      () => {
        clearTimeout(t);
        resolve();
      },
      { once: true },
    );
  });
}
