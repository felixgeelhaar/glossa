/**
 * `<T id="cart.checkout">Zur Kasse</T>`: renders a message, with the children
 * as the inline default (runtimes/SPEC.md §3, step 5), then the message ID.
 * Safe MF2 markup becomes elements by the same rules as `@glossa/elements`;
 * translation text is never rendered as HTML. No wrapper element: plain text
 * renders as a text node.
 */
import { Fragment, createElement } from "react";
import type { ReactNode } from "react";
import { partsToTree, resolveParts } from "@glossa/elements/parts";
import type { TreeNode } from "@glossa/elements/parts";

import { useGlossa } from "./glossa.js";
import type { RegisteredMessages } from "./glossa.js";

export interface TProps<K extends keyof RegisteredMessages & string> {
  /** The message ID. */
  id: K;
  /** The values the message is formatted with. */
  values?: RegisteredMessages[K];
  /** The inline default, rendered when no locale of the active chain has the message. */
  children?: ReactNode;
}

// Children as arguments, not an array: React wants no keys for those.
const toNodes = (nodes: TreeNode[]): ReactNode[] =>
  nodes.map((n) =>
    typeof n === "string" ? n : createElement(n.tag, null, ...toNodes(n.children)),
  );

/**
 * Every rendering of `<T>` leaves through here. Capture mode (RFC 0004 §3.1)
 * will hook in at this point to put `data-glossa-id` and `data-glossa-locale`
 * on a host element while a capture or editor session is active; outside a
 * session it stays the identity, so a normal page view has no extra markup.
 */
const host = (_id: string, content: ReactNode): ReactNode => content;

export function T<K extends keyof RegisteredMessages & string>(props: TProps<K>): ReactNode {
  const { id, values, children } = props;
  const parts = resolveParts(useGlossa(), id, values as Record<string, unknown> | undefined);
  if (!parts) return host(id, children ?? id);
  const tree = partsToTree(parts);
  return host(
    id,
    tree.every((n) => typeof n === "string")
      ? tree.join("")
      : createElement(Fragment, null, ...toNodes(tree)),
  );
}
