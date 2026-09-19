/**
 * A capture or editor session (RFC 0004 §3.1): installs an `onRender` hook on
 * the page's runtimes that marks every `t()` string, keeps the render log the
 * markers point into, collects regions, and ends by removing the hook and the
 * markers it left behind.
 */
import type { Render, Runtime } from "@glossa/runtime";

import { hasMarkers, mark, strip } from "./markers.js";
import { collectRegions } from "./regions.js";
import type { Capture, LogEntry } from "./regions.js";

/** One entry of the session's render log. */
export interface LoggedRender extends LogEntry {
  /** FNV-1a (32-bit, hex) of the values' JSON: equal values, equal digest. Values never enter the log. */
  digest: string;
}

export interface CaptureSession {
  /** The render log: a marker's index is a position in it. Equal renders share an entry. */
  readonly renders: readonly LoggedRender[];
  /** Regions under `root` (default: the document), with the log entries they refer to. */
  collect(root?: Document | Element | ShadowRoot): Capture;
  /**
   * End the session: remove the hook (so the page re-renders without
   * markers) and strip the markers still in the document.
   */
  stop(): void;
}

/** FNV-1a over the values' JSON. BigInts count as their digits; unserializable values digest as `!`. */
export function digest(values: Record<string, unknown> | undefined): string {
  let json: string;
  try {
    json = JSON.stringify(values ?? {}, (_, v) => (typeof v === "bigint" ? `${v}n` : v));
  } catch {
    return "!";
  }
  let h = 0x811c9dc5;
  for (let i = 0; i < json.length; i++) h = Math.imul(h ^ json.charCodeAt(i), 0x01000193);
  return (h >>> 0).toString(16).padStart(8, "0");
}

/**
 * Start a session on the page's runtimes (several for independent islands;
 * they share one log). Their components re-render with host attributes and
 * their `t()` strings with markers.
 */
export function startCapture(runtimes: Runtime | readonly Runtime[]): CaptureSession {
  const renders: LoggedRender[] = [];
  const seen = new Map<string, number>();
  const hook = (r: Render) => {
    const d = digest(r.values);
    const key = JSON.stringify([r.id, r.locale ?? null, d, r.output]);
    let index = seen.get(key);
    if (index === undefined) {
      index = renders.push({ id: r.id, locale: r.locale, digest: d }) - 1;
      seen.set(key, index);
    }
    return mark(index, r.output);
  };
  const offs = (Array.isArray(runtimes) ? runtimes : [runtimes as Runtime]).map((rt) =>
    rt.onRender(hook),
  );
  let active = true;
  return {
    renders,
    collect: (root) => collectRegions(renders, root),
    stop() {
      if (!active) return;
      active = false;
      for (const off of offs) off();
      if (typeof document !== "undefined") stripMarkers(document);
    },
  };
}

/**
 * Remove every marker under `root` (default: the document): text, attribute
 * values, input values and the title, open shadow roots included.
 * Frameworks re-render without markers once the hook is gone; this cleans up
 * what they don't own.
 */
export function stripMarkers(root: Document | Element | ShadowRoot = document): void {
  const doc = root.nodeType === Node.DOCUMENT_NODE ? (root as Document) : root.ownerDocument!;
  const clean = (tree: Node) => {
    const walker = doc.createTreeWalker(tree, NodeFilter.SHOW_ELEMENT | NodeFilter.SHOW_TEXT);
    for (let n: Node | null = walker.currentNode; n; n = walker.nextNode()) {
      if (n.nodeType === Node.TEXT_NODE) {
        const t = n as Text;
        if (hasMarkers(t.data)) t.data = strip(t.data);
        continue;
      }
      if (n.nodeType !== Node.ELEMENT_NODE) continue;
      const el = n as Element;
      for (const a of Array.from(el.attributes)) {
        if (hasMarkers(a.value)) a.value = strip(a.value);
      }
      if (
        (el instanceof HTMLInputElement || el instanceof HTMLTextAreaElement) &&
        hasMarkers(el.value)
      ) {
        el.value = strip(el.value);
      }
      if (el.shadowRoot) clean(el.shadowRoot);
    }
  };
  clean(root);
}
