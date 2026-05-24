---
"@felixgeelhaar/glossa-cli": minor
---

`glossa init` is now interactive by default — prompts for API URL, project slug, locales, and API key. The previous non-interactive path remains available via `glossa init --yes` (CI-friendly) or programmatic callers that pass flags directly. Auto-skips prompts when stdin isn't a TTY so existing scripts keep working without a flag flip.
