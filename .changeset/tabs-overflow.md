---
"@felixgeelhaar/glossa-ui": minor
---

`<gl-tabs>` now supports an overflow `More ▾` group. Items can opt in with `group: "more"` and are rendered inside a popover menu instead of inline; the trigger reflects an aria-current state when the active tab lives inside it. Closes on Esc, click outside, or selection. Existing call sites (no `group`) keep the previous inline-only behavior.
