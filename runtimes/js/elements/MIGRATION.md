# Migrating from v0.3 (`@felixgeelhaar/glossa-elements`)

Templates don't change. `<glossa-text key>`, `<glossa-rich key vars>`,
`<glossa-plural key count>`, `<glossa-select key value name>` and
`<glossa-selector locales labels label auto-detect disabled>` keep their names,
attributes and slot-as-default behaviour. What changes is where the provider
gets its messages: the delivery plane (a signed, cached release from the
edge) instead of the v0.3 API.

## 1. Swap the dependency

```diff
- "@felixgeelhaar/glossa-elements": "^0.2.0",
+ "@glossa/elements": "…",
```

```diff
- import "@felixgeelhaar/glossa-elements";
+ import "@glossa/elements";
```

## 2. Change the provider's attributes

```diff
  <glossa-provider
-   project="kraftsport"
-   api-url="https://glossa.example.com/api/v1"
-   api-key="glossa_…"
+   edge="https://edge.glossa.example.com"
+   delivery-key="pk_…"
    locale="de"
  >
```

| v0.3 | now |
|---|---|
| `project` | gone: the delivery key is scoped to one project |
| `api-url` | `edge` (the delivery plane, not the API) |
| `api-key` (a secret bearer token in the page) | `delivery-key` (publishable and read-only by design) |
| — | `environment` (default `production`), `public-keys` (signature verification), `inspect` |
| `locale`, `strict` | unchanged; `locale` also takes a preference list (`de-AT,de`) |

With `strict`, a provider that still has `api-url` and no `edge` logs a
warning naming the new attributes.

## 3. Import the messages

Run the v0.3 importer so the project's messages (converted from ICU MF1 to
MessageFormat 2) are in the platform, then publish a release. Plural and
select messages keep working: `{count, plural, …}` becomes a `.match` on
`$count`, and `{value, select, …}` a `.match` on `$value`, which is exactly
what `<glossa-plural count>` and `<glossa-select value>` pass.

## What behaves differently

- **Loading and offline.** Messages come from the release: memory, then the
  last good release persisted in `localStorage`, then the edge, then a
  bundled release, then the inline default. A network failure keeps the last
  good release on screen instead of falling back to the defaults.
- **Live updates.** v0.3 patched strings over SSE. Now the provider picks up
  new releases in the background (every 5 minutes and when the tab becomes
  visible). Live editing comes back with the in-product editor.
- **`<glossa-selector>`** lists the release's locales on its own when
  `locales` is omitted (v0.3 showed the current locale read-only), labelled in
  their own language. A pick now switches the provider unless a listener calls
  `preventDefault()` on `glossa-locale-change`. Listeners that set the
  provider's `locale` themselves keep working.
- **Markup.** Safe MF2 markup (`{#b}…{/b}`) renders as elements. Translations
  are still never parsed as HTML.
- **Errors** are `glossa-error` events on the provider (v0.3 only logged them
  in strict mode).
- **Removed:** the `fetchImpl` test seam (use `provider.options = { transport }`)
  and `GlossaContextValue.get()`/`version` (the context now carries the
  runtime).

## `<glossa-text key>` inside `.vue` files

Vue treats `key` as its own vnode key and never renders it as an attribute,
so a `<glossa-text key="…">` in a `.vue` template has no message ID in the
DOM and shows only its inline default, with v0.3 as well as now. Rename the
attribute to `message` there (a mechanical `key=` → `message=` on `glossa-*`
tags in `.vue` files), or switch to `<GlossaText id>` from `@glossa/vue`.
`.astro` files and plain HTML keep `key`.

## Static pages (Astro)

With `@glossa/astro`, `<glossa-*>` elements are rendered at build time, so the
static HTML is already translated and the elements take over without a
flicker. Inside Vue components, keep telling Vue that `glossa-*` tags are
custom elements (`compilerOptions.isCustomElement`), as with v0.3.
