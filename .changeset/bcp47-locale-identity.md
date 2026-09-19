---
"@felixgeelhaar/glossa-sdk": minor
---

Locales are full BCP 47 tags now. `describeLocale(tag)` returns the canonical code, the explicit language, script and region subtags, and the text direction (`ltr`/`rtl`, from the explicit or CLDR likely script). The `Locale` wire type describes `GET /locales` items, which now carry the same fields. The bundle cache keys on the canonical code the server returns, so a bundle requested as `de-de` is also found as `de-DE` and patched by SSE events. `TranslationStatus` now includes `ai_translated`, which the server has been sending since 0.2.
