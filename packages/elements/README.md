# `@felixgeelhaar/glossa-elements`

Framework-agnostic Lit web components for [Glossa](https://github.com/felixgeelhaar/glossa). Drop `<glossa-text>` / `<glossa-rich>` / `<glossa-plural>` / `<glossa-select>` into any Vue / React / Svelte / Astro / plain-HTML page. ~500 LOC, ~10 KB unpacked.

```bash
pnpm add @felixgeelhaar/glossa-elements
```


> **Vue templates:** use `message="…"` instead of `key="…"`. Vue reserves `key` and never renders it as an attribute, so a `key` written in a `.vue` template never reaches the element and only the fallback slot shows. Both attributes work everywhere else; `message` wins when both are set.

## Usage

```html
<script type="module">
  import "@felixgeelhaar/glossa-elements";
</script>

<glossa-provider
  project="brotwerk-site"
  locale="de"
  api-url="https://glossa.example.com/api/v1"
  api-key="glossa_..."
>
  <glossa-text key="hero.title">Brotwerk</glossa-text>

  <glossa-rich key="athlete.greeting" .vars=${{ name: "Sophia" }}>
    Hi, ${name}!
  </glossa-rich>

  <glossa-plural key="session_count" count="3">
    no sessions
  </glossa-plural>

  <glossa-select key="cta_label" choice="founder">
    Sign up
  </glossa-select>
</glossa-provider>
```

## Components

| Tag | Purpose |
|---|---|
| `<glossa-provider>` | Root context — owns the SDK client, current locale, and the live SSE subscription. Children read from a Lit `context` so they update reactively. |
| `<glossa-text>` | Simple key lookup. Slot content is the fallback. |
| `<glossa-rich>` | ICU-formatted key — variable interpolation. Pass `.vars=${{...}}` (property, not attribute). |
| `<glossa-plural>` | Plural-form lookup keyed by `count`. |
| `<glossa-select>` | Select-form lookup keyed by `choice`. |

## Fallback semantics

If a key is missing or the API is unreachable, the slot content renders. Pages stay readable offline; the only thing lost is the localized override.

## Light DOM

Components render their resolved text into light DOM (not shadow DOM) so the surrounding stylesheet styles them like any other text node. No theme leakage; no `:host` quirks.

## Companion packages

- `@felixgeelhaar/glossa-sdk` — the HTTP + SSE client this package wraps.
- `@felixgeelhaar/glossa-format` — the ICU formatter `<glossa-rich>` / `<glossa-plural>` / `<glossa-select>` depend on.

## License

MIT
