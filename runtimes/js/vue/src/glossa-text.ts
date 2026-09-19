/**
 * `<GlossaText id="cart.checkout">Zur Kasse</GlossaText>`: renders a message,
 * with the default slot as the inline default (runtimes/SPEC.md §3, step 5),
 * then the message ID. Safe MF2 markup becomes elements by the same rules as
 * `@glossa/elements`; translation text is never rendered as HTML. No wrapper
 * element: plain text renders as a text node.
 */
import { defineComponent, h } from "vue";
import type { PropType, VNodeChild } from "vue";
import { partsToTree, resolveParts } from "@glossa/elements/parts";
import type { TreeNode } from "@glossa/elements/parts";

import { useState } from "./glossa.js";

const toVNodes = (nodes: TreeNode[]): VNodeChild[] =>
  nodes.map((n) => (typeof n === "string" ? n : h(n.tag, toVNodes(n.children))));

export const GlossaText = defineComponent({
  name: "GlossaText",
  props: {
    /** The message ID. */
    id: { type: String, required: true },
    /** The values the message is formatted with. */
    values: { type: Object as PropType<Record<string, unknown>>, default: undefined },
  },
  setup(props, { slots }) {
    const { runtime, version } = useState();
    return () => {
      void version.value;
      const parts = resolveParts(runtime, props.id, props.values);
      if (!parts) return slots.default ? slots.default() : props.id;
      const tree = partsToTree(parts);
      return tree.every((n) => typeof n === "string") ? tree.join("") : toVNodes(tree);
    };
  },
});
