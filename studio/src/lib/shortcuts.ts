/**
 * Studio's keyboard map. This list is the single source of truth: the
 * `?` sheet renders it, the handlers match against it, and a test keeps
 * studio/README.md in step with it.
 */
import { onBeforeUnmount, onMounted } from "vue";

export type ShortcutId = "help" | "search" | "next" | "prev" | "edit" | "leave" | "save" | "saveApprove" | "publish";

export interface Shortcut {
  id: ShortcutId;
  /** Key caps as shown to people; "Mod" renders as ⌘ on Apple platforms and Ctrl elsewhere. */
  keys: string[];
  description: string;
  /** Where it applies. */
  context: "Everywhere" | "Translator workspace" | "Translation editor" | "Releases";
  /** Fires while focus is in a text field (only chords with a modifier or Escape do). */
  inText: boolean;
  match: (e: KeyboardEvent) => boolean;
}

const mod = (e: KeyboardEvent) => e.metaKey || e.ctrlKey;
const plain = (e: KeyboardEvent) => !e.metaKey && !e.ctrlKey && !e.altKey;

export const SHORTCUTS: readonly Shortcut[] = [
  {
    id: "help", keys: ["?"], description: "Show keyboard shortcuts", context: "Everywhere", inText: false,
    match: (e) => e.key === "?" && !e.metaKey && !e.ctrlKey && !e.altKey,
  },
  {
    id: "search", keys: ["/"], description: "Search messages", context: "Translator workspace", inText: false,
    match: (e) => e.key === "/" && plain(e),
  },
  {
    id: "next", keys: ["j"], description: "Next message", context: "Translator workspace", inText: false,
    match: (e) => e.key === "j" && plain(e) && !e.shiftKey,
  },
  {
    id: "prev", keys: ["k"], description: "Previous message", context: "Translator workspace", inText: false,
    match: (e) => e.key === "k" && plain(e) && !e.shiftKey,
  },
  {
    id: "edit", keys: ["Enter"], description: "Edit the translation", context: "Translator workspace", inText: false,
    // Enter still activates a focused link, button or tab.
    match: (e) => e.key === "Enter" && plain(e) && !e.shiftKey && !isActivatable(e.target),
  },
  {
    id: "leave", keys: ["Esc"], description: "Leave the editor, back to the message list", context: "Translation editor",
    inText: true, match: (e) => e.key === "Escape" && plain(e),
  },
  {
    id: "save", keys: ["Mod", "Enter"], description: "Save the translation", context: "Translation editor", inText: true,
    match: (e) => e.key === "Enter" && mod(e) && !e.shiftKey && !e.altKey,
  },
  {
    id: "saveApprove", keys: ["Mod", "Shift", "Enter"], description: "Save and approve", context: "Translation editor",
    inText: true, match: (e) => e.key === "Enter" && mod(e) && e.shiftKey && !e.altKey,
  },
  {
    id: "publish", keys: ["p"], description: "Publish a release", context: "Releases", inText: false,
    match: (e) => e.key === "p" && plain(e) && !e.shiftKey,
  },
];

export const isApple = (): boolean => /Mac|iPhone|iPad/.test(globalThis.navigator?.platform ?? "");

/** How a key cap is shown on this platform. */
export function keyLabel(key: string, apple = isApple()): string {
  if (key === "Mod") return apple ? "⌘" : "Ctrl";
  if (key === "Shift") return apple ? "⇧" : "Shift";
  if (key === "Enter") return apple ? "↵ Return" : "Enter";
  return key;
}

/** Is focus somewhere that takes text? */
export function isTextTarget(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false;
  if (target.isContentEditable) return true;
  if (target instanceof HTMLTextAreaElement || target instanceof HTMLSelectElement) return true;
  if (target instanceof HTMLInputElement) {
    return !["checkbox", "radio", "button", "submit", "reset", "range", "color", "file"].includes(target.type);
  }
  return false;
}

/** Does Enter already mean something on this element? */
export function isActivatable(target: EventTarget | null): boolean {
  return target instanceof HTMLElement && target.closest("a[href], button, summary, [role='button'], [role='tab'], [role='link']") !== null;
}

/** The shortcut an event triggers, if any. */
export function matchShortcut(e: KeyboardEvent, enabled: ReadonlySet<ShortcutId>): Shortcut | undefined {
  if (e.isComposing || e.defaultPrevented) return undefined;
  const inText = isTextTarget(e.target);
  return SHORTCUTS.find((s) => enabled.has(s.id) && (s.inText || !inText) && s.match(e));
}

/** A handler returns `false` when the key isn't for it, leaving the event to others. */
export type ShortcutHandlers = Partial<Record<ShortcutId, (e: KeyboardEvent) => void | false>>;

/** Bind handlers for the lifetime of the calling component. */
export function useShortcuts(handlers: ShortcutHandlers): void {
  const enabled = new Set(Object.keys(handlers) as ShortcutId[]);
  const onKey = (e: KeyboardEvent) => {
    const s = matchShortcut(e, enabled);
    if (!s) return;
    // A modal owns the keyboard; only the sheet's own toggle passes through.
    if (s.id !== "help" && document.querySelector("dialog[open]")) return;
    if (handlers[s.id]?.(e) !== false) e.preventDefault();
  };
  onMounted(() => window.addEventListener("keydown", onKey));
  onBeforeUnmount(() => window.removeEventListener("keydown", onKey));
}
