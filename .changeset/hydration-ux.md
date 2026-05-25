---
"@felixgeelhaar/glossa-elements": patch
---

`<glossa-text>` now surfaces a hydration state on the host element so SSR fallback content is visually distinct from the live-resolved value: `aria-busy="true"` + `data-glossa-pending` while the provider's first bundle is in flight, `data-glossa-missing` when the key is genuinely absent post-hydration. Default styles dim pending content slightly and dotted-outline missing content; consumers can override via `::slotted()` selectors on their own page CSS.
