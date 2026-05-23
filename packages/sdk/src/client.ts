// HTTP client: bundle() + scan(). Tiny on purpose — fetch only,
// no axios/superagent, so the package stays under its 5 KB
// size-limit budget.
//
// Caching model: bundle() keeps the last successful response per
// locale alongside the ETag the server returned. On the next call
// we send `If-None-Match: <etag>`; a 304 means our cached copy is
// still fresh and we hand it back. Any 2xx replaces the cache.
//
// The cache also exposes a hook the SSE subscription uses to
// surgically patch a single key without invalidating the whole
// bundle.

import { subscribe, type SubscribeOptions, type Subscription } from "./subscribe.js";
import type {
  Bundle,
  ClientConfig,
  ScanInput,
  ScanResponse,
  TranslationUpdatedEvent,
} from "./types.js";

/** Public client surface. */
export interface Client {
  /**
   * Fetch the message bundle for a locale. Subsequent calls send
   * `If-None-Match` with the previous ETag; a 304 response returns
   * the cached bundle unchanged.
   */
  bundle(locale: string): Promise<Bundle>;

  /**
   * Look up a single message by key in the cached bundle. Returns
   * `undefined` on a miss — callers fall back to the key itself or
   * the default-locale bundle.
   */
  message(locale: string, key: string): string | undefined;

  /**
   * Open an SSE subscription for the project. The handler fires
   * once per `translation.updated` event; the SDK also patches its
   * in-memory cache so the next `message()` call reflects the
   * change without an extra round-trip.
   */
  subscribe(opts: SubscribeOptions): Subscription;

  /**
   * Batch UPSERT translation keys. Used by the CLI's `glossa scan`
   * command to register newly-discovered keys.
   */
  scan(keys: ScanInput[]): Promise<ScanResponse>;
}

interface CachedBundle {
  bundle: Bundle;
  etag: string | undefined;
}

const BEARER = "Bearer ";

/** Construct a Client. Pure factory — no I/O at construction time. */
export function createClient(config: ClientConfig): Client {
  if (!config.project) throw new Error("createClient: project required");
  if (!config.apiKey) throw new Error("createClient: apiKey required");
  if (!config.apiUrl) throw new Error("createClient: apiUrl required");

  const apiUrl = config.apiUrl.replace(/\/+$/, "");
  const doFetch = config.fetch ?? fetch;
  const cache = new Map<string, CachedBundle>();

  const projectBase = `${apiUrl}/api/v1/projects/${encodeURIComponent(config.project)}`;

  function authHeaders(extra?: HeadersInit): Headers {
    const h = new Headers(extra);
    h.set("Authorization", BEARER + config.apiKey);
    return h;
  }

  async function bundle(locale: string): Promise<Bundle> {
    const cached = cache.get(locale);
    const headers = authHeaders({ Accept: "application/json" });
    if (cached?.etag) headers.set("If-None-Match", cached.etag);

    const url = `${projectBase}/locales/${encodeURIComponent(locale)}/messages`;
    const res = await doFetch(url, { method: "GET", headers });

    if (res.status === 304 && cached) {
      return cached.bundle;
    }
    if (!res.ok) {
      throw new GlossaError(`bundle: ${res.status} ${res.statusText}`, res.status);
    }
    const body = (await res.json()) as Bundle;
    cache.set(locale, { bundle: body, etag: res.headers.get("ETag") ?? undefined });
    return body;
  }

  function message(locale: string, key: string): string | undefined {
    return cache.get(locale)?.bundle.messages[key];
  }

  // applyUpdate is shared between subscribe()'s onEvent callback
  // (cache patch) and would also fit any future REST mutation that
  // wanted to keep the cache hot. Kept un-exported — the public
  // patch path is via SSE.
  function applyUpdate(e: TranslationUpdatedEvent): void {
    const c = cache.get(e.locale);
    if (!c) return; // never fetched this locale → nothing to patch
    c.bundle.messages[e.key] = e.value;
    c.bundle.statuses[e.key] = e.status;
  }

  function subscribeFn(opts: SubscribeOptions): Subscription {
    return subscribe({
      url: `${projectBase}/sse`,
      apiKey: config.apiKey,
      fetch: doFetch,
      onEvent: (e) => {
        applyUpdate(e);
        opts.onEvent?.(e);
      },
      onError: opts.onError,
      onOpen: opts.onOpen,
      signal: opts.signal,
    });
  }

  async function scan(keys: ScanInput[]): Promise<ScanResponse> {
    const url = `${projectBase}/keys:scan`;
    const headers = authHeaders({ "Content-Type": "application/json" });
    const res = await doFetch(url, {
      method: "POST",
      headers,
      body: JSON.stringify({ keys }),
    });
    if (!res.ok) {
      throw new GlossaError(`scan: ${res.status} ${res.statusText}`, res.status);
    }
    return (await res.json()) as ScanResponse;
  }

  return { bundle, message, subscribe: subscribeFn, scan };
}

/**
 * Thrown by client methods when the server returns a non-2xx
 * status. Carries the HTTP code so callers can distinguish 401
 * (bad key) from 5xx (transient — worth retrying).
 */
export class GlossaError extends Error {
  public readonly status: number;
  public constructor(message: string, status: number) {
    super(message);
    this.name = "GlossaError";
    this.status = status;
  }
}
