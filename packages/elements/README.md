# `@glossa/elements` — Lit web components

Stub. Implementation lands after `@glossa/format` + `@glossa/sdk` are ready.

## Public surface (planned)

```html
<glossa-provider
  project="iri"
  locale="de"
  api-url="https://glossa.example/api/v1"
  api-key="..."
  strict>
  <glossa-text key="coach.plan.approve">Approve plan</glossa-text>
  <glossa-rich
    key="athlete.greeting"
    vars='{"name":"Sophia"}'>Hi, ${name}!</glossa-rich>
  <glossa-plural
    key="athlete.session_count"
    count="${count}">no sessions</glossa-plural>
</glossa-provider>
```

## Principles

- **Fallback always wins.** When the key is missing or the API is unreachable, slot content renders. Apps still work offline.
- **Framework-agnostic.** Drops into Vue, React, Svelte, Astro, plain HTML.
- **Smallest possible runtime.** Network code lives in `@glossa/sdk`; formatting in `@glossa/format`. This package is pure rendering + reactivity.
