# `@felixgeelhaar/glossa-ui`

Lit primitives + design tokens that power the [Glossa](https://github.com/felixgeelhaar/glossa) admin SPA. Light / dark / system theming via CSS custom properties; form-associated where it matters (works inside native `<form>`). ~1100 LOC, ~75 KB unpacked.

```bash
pnpm add @felixgeelhaar/glossa-ui
```

## Usage

```ts
import "@felixgeelhaar/glossa-ui/tokens.css";
import { initTheme } from "@felixgeelhaar/glossa-ui";
import "@felixgeelhaar/glossa-ui";

initTheme(); // applies persisted theme before first paint
```

```html
<gl-button variant="primary">Save</gl-button>
<gl-input label="Email" type="email" required></gl-input>
<gl-badge variant="approved">approved</gl-badge>
```

## Primitives

| Tag | Notes |
|---|---|
| `gl-button` | variants: `primary` / `outline` / `ghost` / `danger`; sizes: `sm` / `md` |
| `gl-input` | form-associated via `ElementInternals.setFormValue` |
| `gl-select` | form-associated |
| `gl-textarea` | form-associated |
| `gl-card` | optional `header` slot, `flush` attribute for tables |
| `gl-badge` | variants: `pending` / `review` / `approved` / `danger` / `accent` / `neutral` |
| `gl-table` + `glTableStyles` | shared CSS for hand-rolled tables that want gl-table look |
| `gl-tabs` | emits `gl-tab-change` with `{ id }` |
| `gl-toast` + `toast()` | one-shot notifications, auto-dismiss |
| `gl-toolbar` | top-bar with title + center + actions slots, brand mark built in |
| `gl-theme-toggle` | system → light → dark cycle |

## Theming

Switch theme via the `data-glossa-theme` attribute on `<html>` or `<body>`:

```html
<html data-glossa-theme="dark">
```

`initTheme()` reads the persisted preference (or `prefers-color-scheme` if "system") and applies it before first paint to prevent flash. `setTheme()` and `getTheme()` are exposed for theme toggles.

Token names follow `--gl-<group>-<role>`: `--gl-bg`, `--gl-surface`, `--gl-text`, `--gl-text-dim`, `--gl-accent`, `--gl-danger`, etc. See `src/tokens.css` for the full set.

## Inter font

`tokens.css` references Inter as the UI font. Either load it yourself (`<link>` to rsms.me) or override `--gl-font-ui`.

## License

MIT
