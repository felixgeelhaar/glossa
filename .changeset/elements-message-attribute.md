---
"@felixgeelhaar/glossa-elements": minor
---

`<glossa-text>`, `<glossa-rich>`, `<glossa-plural>` and `<glossa-select>` accept `message="…"` as well as `key="…"`. Vue reserves `key` for its own reconciliation and never renders it as a DOM attribute, so inside `.vue` templates `<glossa-text key="…">` reached the element without a message ID and always showed its slot fallback, whatever the locale. Use `message` in Vue templates; `key` keeps working everywhere else, and `message` wins when both are set.
