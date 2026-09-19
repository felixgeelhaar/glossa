/**
 * The runtime contract (runtimes/SPEC.md §3–§6): load a release from memory,
 * persisted last-good, the edge or the bundle; resolve the locale and its
 * fallback chain; format; explain. Nothing here throws into application
 * code: failures fall through to the next source and go to the error channel.
 */
import { format, formatToParts } from "./format.js";
import type { FormatOptions, Part } from "./format.js";
import { canonicalLocales, fallbackChain, lookupLocale, navigatorLanguages } from "./locale.js";
import type { Artifact, Manifest } from "./manifest.js";
import type { Message } from "./model.js";
import { webStorage } from "./storage.js";
import type { RuntimeStorage } from "./storage.js";
import { checkManifest, sha256Hex, verifySignature } from "./verify.js";
import type { PublicKey } from "./verify.js";

/** Where the rendered text came from (SPEC §6). */
export type Source = "memory" | "persisted" | "network" | "bundled" | "inline";

/** An entry on the error channel (SPEC §6). */
export interface RuntimeError {
  type: "network" | "integrity" | "signature" | "schema" | "format" | "missing-message";
  detail: string;
  messageId?: string;
  locale?: string;
  releaseId?: string;
}

/** The subset of a `fetch` Response the runtime reads. */
export interface TransportResponse {
  status: number;
  headers: { get(name: string): string | null };
  text(): Promise<string>;
}

/** Fetches a URL; `fetch` itself is one. Rejections count as network errors. */
export type Transport = (
  url: string,
  init: { headers: Record<string, string> },
) => Promise<TransportResponse>;

/** A release shipped with the build (`glossa pull --release`), artifacts keyed by SHA-256. */
export interface BundledRelease {
  manifest: Manifest;
  artifacts: Record<string, Artifact>;
}

/** What storage holds: the last release that loaded and verified completely. */
export interface PersistedRelease {
  etag?: string;
  manifest: Manifest;
  /** SHA-256 → the artifact's exact bytes. */
  artifacts: Record<string, string>;
}

export interface RuntimeOptions {
  /** Edge origin, e.g. `https://edge.example.com`. With `deliveryKey`, enables network loading. */
  edge?: string;
  /** The project's publishable delivery key. */
  deliveryKey?: string;
  /** Default `"production"`. */
  environment?: string;
  /** Requested locales, most preferred first, e.g. `resolveLocales(…)`. Default: the browser's. */
  locales?: string | readonly string[];
  /** Catalogs shipped with the build: rendered synchronously, and the last resort offline. */
  bundled?: BundledRelease;
  /** Trusted signing keys. When set, manifests without a valid signature are rejected. */
  publicKeys?: readonly PublicKey[];
  /** Where last-good persists. Default: `localStorage` in browsers; `null` disables it. */
  storage?: RuntimeStorage | null;
  /** Default: `fetch`. */
  transport?: Transport;
  /** Background manifest refresh in ms. Default 5 minutes; `0` disables it. */
  refreshInterval?: number;
  /** Schedules the background refresh and returns a cancel function. Default: `setInterval`. */
  timer?: (tick: () => void, ms: number) => () => void;
  /** Bidi isolation of placeholders; the MF2 default (`"default"`) isolates them. */
  bidiIsolation?: FormatOptions["bidiIsolation"];
  /** Custom MF2 functions, merged over the built-ins. */
  functions?: FormatOptions["functions"];
  /** Error channel listener; more can be added with `onError()`. */
  onError?: (error: RuntimeError) => void;
  /** An identical error is reported at most once per this many ms. Default 60 s. */
  errorInterval?: number;
}

export interface TranslateOptions {
  /** Inline default rendered when no locale in the chain has the message. */
  default?: string;
}

export interface ExplainStep {
  locale: string;
  outcome: "found" | "missing" | "not-loaded";
}

/** Why a message renders the way it does (SPEC §6). */
export interface Explanation {
  id: string;
  requested: string[];
  locale: string | null;
  chain: string[];
  /** `null` when the inline default (or the message ID) is used. */
  resolvedFrom: string | null;
  release: { id: string; version: number } | null;
  source: Source;
  steps: ExplainStep[];
}

export interface Runtime {
  /** Settles when the first load (persisted, then network) is done. Never rejects. */
  readonly ready: Promise<void>;
  /** The active locale, once a release is active. */
  readonly locale: string | undefined;
  /** The active locale's direction, for `dir` attributes. */
  readonly dir: "ltr" | "rtl";
  readonly release: { id: string; version: number } | undefined;
  /** Render a message as a string. */
  t(id: string, values?: Record<string, unknown>, opts?: TranslateOptions): string;
  /** Render a message as parts (text, markup, bidi isolates, fallbacks, values). */
  parts(id: string, values?: Record<string, unknown>, opts?: TranslateOptions): Part[];
  /** How `id` resolves for the active (or the given) locales. Loads nothing. */
  explain(id: string, locales?: string | readonly string[]): Explanation;
  /** Switch the requested locales; the switch is atomic once their artifacts are loaded. */
  setLocales(locales: string | readonly string[]): Promise<void>;
  /** Revalidate the manifest now. Concurrent calls share one request. */
  refresh(): Promise<void>;
  /** Called after every activation (new release or locale), e.g. to re-render. */
  subscribe(listener: () => void): () => void;
  /** Listen on the error channel. */
  onError(listener: (error: RuntimeError) => void): () => void;
  /** Stop background refresh and drop listeners. */
  dispose(): void;
}

interface Active {
  m: Manifest;
  etag?: string;
  source: Source;
  requested: string[];
  locale: string;
  chain: string[];
  catalogs: Array<[string, Record<string, Message>]>;
}

const list = (l: string | readonly string[]) => canonicalLocales(typeof l === "string" ? [l] : l);

const shas = (m: Manifest, locale: string) =>
  Object.values((Object.hasOwn(m.artifacts, locale) && m.artifacts[locale]) || {}).map(
    (a) => a.sha256,
  );

/** The active locale and its fallback chain for `requested` (SPEC §4). */
const plan = (m: Manifest, requested: string[]): [string, string[]] => {
  const locale =
    lookupLocale(
      requested,
      m.locales.map((l) => l.code),
    ) ?? m.sourceLocale;
  return [locale, fallbackChain(locale, m)];
};

/** Create a runtime. Everything is optional: with nothing configured it renders inline defaults. */
export function createRuntime(o: RuntimeOptions = {}): Runtime {
  const keys = o.publicKeys ?? [];
  const transport: Transport = o.transport ?? ((url, init) => fetch(url, init));
  const base = o.edge && o.deliveryKey && `${o.edge.replace(/\/+$/, "")}/v1/${o.deliveryKey}`;
  const env = o.environment ?? "production";
  const storeKey = `glossa:${o.deliveryKey}:${env}`;
  let storage = o.storage;
  if (storage === undefined && "document" in globalThis) {
    try {
      storage = webStorage();
    } catch {
      // No localStorage (sandboxed frame, disabled storage): memory only.
    }
  }
  const cache = new Map<string, { a: Artifact; text?: string }>();
  const subscribers = new Set<() => void>();
  const listeners = new Set<(e: RuntimeError) => void>(o.onError ? [o.onError] : []);
  const reported = new Map<string, number>();
  let requested = list(o.locales ?? navigatorLanguages() ?? []);
  let state: Active | undefined;
  let record: Promise<PersistedRelease | undefined> | undefined;
  let queue: Promise<unknown> = Promise.resolve();
  let inflight: Promise<void> | undefined;

  const call = <A extends unknown[]>(fs: Iterable<(...a: A) => void>, ...args: A) => {
    for (const f of fs) {
      try {
        f(...args);
      } catch {
        // A listener's bug must not break loading or rendering.
      }
    }
  };

  const emit = (e: RuntimeError) => {
    const key = JSON.stringify(e);
    const now = Date.now();
    if (now - (reported.get(key) ?? -Infinity) < (o.errorInterval ?? 60_000)) return;
    if (reported.size > 999) reported.clear();
    reported.set(key, now);
    call(listeners, e);
  };

  /** Activations run one at a time, so the last one to commit is the last one asked for. */
  const run = (task: () => Promise<unknown>) =>
    (queue = queue.then(task).catch(() => {})) as Promise<void>;

  const persisted = () =>
    (record ??= (async () => {
      try {
        return ((await storage?.get(storeKey)) ?? undefined) as PersistedRelease | undefined;
      } catch {
        return undefined;
      }
    })());

  const persist = async (m: Manifest, etag?: string) => {
    const old = (await persisted())?.artifacts ?? {};
    const artifacts: Record<string, string> = {};
    for (const l of Object.keys(m.artifacts)) {
      for (const s of shas(m, l)) {
        const text = cache.get(s)?.text ?? old[s];
        if (text !== undefined) artifacts[s] = text;
      }
    }
    const r: PersistedRelease = { etag, manifest: m, artifacts };
    record = Promise.resolve(r);
    try {
      await storage?.set(storeKey, r);
    } catch {
      // Quota or storage errors: this process keeps working from memory.
    }
  };

  /** GET a URL. 200 → body and ETag, 304 → no body, anything else → a network error. */
  const get = async (url: string, headers: Record<string, string> = {}, releaseId?: string) => {
    try {
      const res = await transport(url, { headers });
      if (res.status === 304) return {};
      if (res.status !== 200) throw `HTTP ${res.status}`;
      return { text: await res.text(), etag: res.headers.get("etag") ?? undefined };
    } catch (e) {
      emit({ type: "network", detail: `${url}: ${String(e)}`, releaseId });
      return undefined;
    }
  };

  /** Verify an artifact's bytes against its hash and cache it (SPEC §1.3). */
  const accept = async (sha: string, text: string | undefined, releaseId: string) => {
    if (text === undefined) return false;
    if ((await sha256Hex(text)) !== sha) {
      emit({ type: "integrity", detail: `artifact ${sha} doesn't match its hash`, releaseId });
      return false;
    }
    try {
      const a = JSON.parse(text) as Artifact;
      if (!/^glossa\.artifact\/v1\b/.test(a.schema) || typeof a.messages !== "object") throw 0;
      cache.set(sha, { a, text });
      return true;
    } catch {
      emit({ type: "schema", detail: `artifact ${sha} isn't a v1 artifact`, releaseId });
      return false;
    }
  };

  /** One artifact, by the load order of SPEC §3: memory, persisted, network, bundled. */
  const load = async (sha: string, releaseId: string) => {
    if (cache.has(sha)) return true;
    if (await accept(sha, (await persisted())?.artifacts?.[sha], releaseId)) return true;
    if (
      base &&
      (await accept(sha, (await get(`${base}/a/${sha}.json`, {}, releaseId))?.text, releaseId))
    ) {
      return true;
    }
    const b = o.bundled?.artifacts[sha];
    if (b) cache.set(sha, { a: b });
    return !!b;
  };

  /** A locale's messages from the cache, namespaces merged; undefined if one isn't loaded. */
  const catalog = (m: Manifest, locale: string) => {
    const parts = shas(m, locale).map((s) => cache.get(s)?.a.messages);
    return parts.includes(undefined)
      ? undefined
      : (Object.assign({}, ...parts) as Record<string, Message>);
  };

  /** Make `m` active for `req`, if everything its chain needs is cached. */
  const commit = (m: Manifest, source: Source, etag?: string, req = requested) => {
    const [locale, chain] = plan(m, req);
    const catalogs: Active["catalogs"] = [];
    for (const l of chain) {
      const c = catalog(m, l);
      if (!c) return false;
      catalogs.push([l, c]);
    }
    state = { m, etag, source, requested: req, locale, chain, catalogs };
    const keep = new Set(Object.keys(m.artifacts).flatMap((l) => shas(m, l)));
    for (const s of cache.keys()) if (!keep.has(s)) cache.delete(s);
    call(subscribers);
    return true;
  };

  /**
   * Verify a manifest, load every artifact its active chain needs, then switch
   * to it in one step (SPEC §3: atomic activation). On any failure the
   * previous release keeps serving. The requested locales are pinned for the
   * whole activation; a `setLocales()` meanwhile queues its own activation.
   */
  const activate = async (m: Manifest, source: Source, etag?: string) => {
    const req = requested;
    let problem = checkManifest(m);
    let type: RuntimeError["type"] = "schema";
    if (!problem && keys.length && m !== state?.m) {
      problem = await verifySignature(m, keys);
      type = "signature";
    }
    if (problem) {
      emit({ type, detail: problem, releaseId: (m as Partial<Manifest> | null)?.release?.id });
      return false;
    }
    const needed = plan(m, req)[1].flatMap((l) => shas(m, l));
    const loaded = await Promise.all(needed.map((s) => load(s, m.release.id)));
    if (loaded.includes(false) || !commit(m, source, etag, req)) return false;
    if (source !== "bundled") await persist(m, etag);
    return true;
  };

  const cycle = async (initial: boolean) => {
    let activated = false;
    if (initial) {
      const r = await persisted();
      const bundledIsNewer =
        !!o.bundled && o.bundled.manifest.release.version > (r?.manifest?.release?.version ?? 0);
      if (r?.manifest && !bundledIsNewer)
        activated = await activate(r.manifest, "persisted", r.etag);
    }
    if (base) {
      const headers: Record<string, string> = state?.etag ? { "If-None-Match": state.etag } : {};
      const res = await get(`${base}/${env}/manifest.json`, headers);
      if (res?.text !== undefined) {
        let m: Manifest | undefined;
        try {
          m = JSON.parse(res.text) as Manifest;
        } catch {
          // Not JSON: checkManifest reports it as a schema error.
        }
        activated = (await activate(m!, "network", res.etag)) || activated;
      }
    }
    // Nothing new this time: what's active was already in memory.
    if (state && !initial && !activated) state.source = "memory";
  };

  const refresh = () =>
    (inflight ??= run(() => cycle(false)).finally(() => (inflight = undefined)));

  if (o.bundled && !checkManifest(o.bundled.manifest)) {
    for (const [s, a] of Object.entries(o.bundled.artifacts)) cache.set(s, { a });
    commit(o.bundled.manifest, "bundled");
  }
  const ready = run(() => cycle(true));

  const every = o.refreshInterval ?? 300_000;
  const timer =
    o.timer ??
    ((tick: () => void, ms: number) => {
      const h = setInterval(tick, ms) as unknown as { unref?: () => void };
      h.unref?.(); // don't keep a server process alive
      return () => clearInterval(h as unknown as number);
    });
  const stop = base && every > 0 ? timer(() => void refresh(), every) : undefined;
  const doc = (globalThis as { document?: Document }).document;
  const onVisible = () => doc!.visibilityState === "visible" && void refresh();
  if (base) doc?.addEventListener?.("visibilitychange", onVisible);

  /** Resolve `id` along the active chain (SPEC §4.3) and render it with `f`. */
  const render =
    <T>(
      f: (m: Message, l: string, v?: Record<string, unknown>, o?: FormatOptions) => T,
      wrap: (s: string) => T,
    ) =>
    (id: string, values?: Record<string, unknown>, opts?: TranslateOptions): T => {
      try {
        const st = state;
        if (st) {
          const releaseId = st.m.release.id;
          for (const [locale, messages] of st.catalogs) {
            if (!Object.hasOwn(messages, id)) continue;
            return f(messages[id]!, locale, values, {
              bidiIsolation: o.bidiIsolation,
              functions: o.functions,
              onError: (e) =>
                emit({
                  type: "format",
                  detail: `${e.type} ${e.source}`,
                  messageId: id,
                  locale,
                  releaseId,
                }),
            });
          }
          const detail = `not in ${st.chain.join(", ")}`;
          emit({ type: "missing-message", detail, messageId: id, locale: st.locale, releaseId });
        }
      } catch {
        // Fall through to the inline default.
      }
      return wrap(opts?.default || id);
    };

  const explain = (id: string, locales?: string | readonly string[]): Explanation => {
    const st = state;
    const req = locales === undefined ? (st?.requested ?? requested) : list(locales);
    if (!st) {
      const release = null;
      return {
        id,
        requested: req,
        locale: null,
        chain: [],
        resolvedFrom: null,
        release,
        source: "inline",
        steps: [],
      };
    }
    const { m } = st;
    const [locale, chain] = plan(m, req);
    const steps: ExplainStep[] = [];
    let resolvedFrom: string | null = null;
    for (const l of chain) {
      const c = catalog(m, l);
      const found = !!c && Object.hasOwn(c, id);
      steps.push({ locale: l, outcome: !c ? "not-loaded" : found ? "found" : "missing" });
      if (found) {
        resolvedFrom = l;
        break;
      }
    }
    const release = { id: m.release.id, version: m.release.version };
    const source = resolvedFrom ? st.source : "inline";
    return { id, requested: req, locale, chain, resolvedFrom, release, source, steps };
  };

  const add = <T>(set: Set<T>, f: T) => (set.add(f), () => void set.delete(f));

  return {
    ready,
    get locale() {
      return state?.locale;
    },
    get dir() {
      const st = state;
      return st?.m.locales.find((l) => l.code === st.locale)?.direction ?? "ltr";
    },
    get release() {
      return state && { id: state.m.release.id, version: state.m.release.version };
    },
    t: render(format, (s) => s),
    parts: render(formatToParts, (value): Part[] => [{ type: "text", value }]),
    explain,
    setLocales(locales) {
      requested = list(locales);
      return run(async () => {
        if (state) await activate(state.m, state.source, state.etag);
      });
    },
    refresh,
    subscribe: (f) => add(subscribers, f),
    onError: (f) => add(listeners, f),
    dispose() {
      stop?.();
      doc?.removeEventListener?.("visibilitychange", onVisible);
      subscribers.clear();
      listeners.clear();
    },
  };
}
