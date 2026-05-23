// @lit/context value shared between <glossa-provider> and every
// descendant <glossa-text|rich|plural|select>.
//
// The provider replaces the entire object whenever bundle/locale
// state changes — consumers are subscribed via `@lit/context`'s
// ContextConsumer, which only fires on reference change. That's
// why patches go through `setValue(newObject)` rather than mutating
// fields in place.

import { createContext } from "@lit/context";

export interface GlossaContextValue {
  /** Current locale code, e.g. "de" or "pt-BR". */
  locale: string;

  /**
   * Flat-string lookup. Returns `undefined` when the key isn't in
   * the loaded bundle; consumers fall back to slot content.
   */
  get(key: string): string | undefined;

  /**
   * Strict mode flag from the provider. Surfaced so child elements
   * can warn on misses without re-reading the provider attribute
   * via DOM traversal.
   */
  strict: boolean;

  /**
   * Monotonic counter bumped whenever the bundle changes — used
   * internally so reference equality on this object alone is
   * enough to trigger consumer re-renders. Not for application
   * code.
   */
  version: number;
}

export const glossaContext = createContext<GlossaContextValue>(Symbol("glossa"));
