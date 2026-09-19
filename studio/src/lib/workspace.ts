/** Pure helpers for the translator workspace's message list. */
import type { Message } from "../api/schemas";

export type RowStatus = "missing" | "outdated" | "translated" | "unknown";

export interface MessageRow {
  key: string;
  text: string;
  namespace: string;
  status: RowStatus;
}

export type Coverage = "all" | "missing" | "outdated";

/** Status of a message in the target locale, from the missing/outdated key sets (unknown until they load). */
export function statusOf(key: string, missing: ReadonlySet<string> | undefined, outdated: ReadonlySet<string> | undefined): RowStatus {
  if (!missing || !outdated) return "unknown";
  if (missing.has(key)) return "missing";
  if (outdated.has(key)) return "outdated";
  return "translated";
}

/** Case-insensitive search over key, source text and the developers' context. */
export function matches(m: Pick<Message, "key" | "description" | "source">, query: string): boolean {
  const q = query.trim().toLocaleLowerCase();
  if (q === "") return true;
  return (
    m.key.toLocaleLowerCase().includes(q) ||
    m.source.text.toLocaleLowerCase().includes(q) ||
    m.description.toLocaleLowerCase().includes(q)
  );
}

/** Namespaces seen so far, sorted, `default` first. */
export function namespacesOf(messages: readonly Pick<Message, "namespace">[], extra: readonly string[] = []): string[] {
  const set = new Set([...messages.map((m) => m.namespace), ...extra.filter(Boolean)]);
  return [...set].sort((a, b) => (a === "default" ? -1 : b === "default" ? 1 : a.localeCompare(b)));
}
