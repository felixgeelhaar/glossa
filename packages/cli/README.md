# `@felixgeelhaar/glossa-cli`

Build-time tooling for [Glossa](https://github.com/felixgeelhaar/glossa). Walks a source tree, extracts translation keys, syncs them with the API, and pulls bundles to disk for build-time baking. ~620 LOC.

```bash
pnpm add -D @felixgeelhaar/glossa-cli
# or
pnpm dlx @felixgeelhaar/glossa-cli init
```

## Commands

| Command | Purpose |
|---|---|
| `glossa init` | Scaffolds a `glossa.config.json` in the current directory. |
| `glossa scan` | Walks source files, extracts `<glossa-text key="...">` / `<glossa-rich>` / `<glossa-plural>` / `<glossa-select>` keys + JS/TS string-literal keys, POSTs them to `/keys:scan` so they exist before translators see them. |
| `glossa pull` | Fetches every locale bundle to disk (default `./glossa/bundles/<locale>.json`). Use these as static fallbacks the app loads when Glossa is offline. |
| `glossa push` | Pushes a translated bundle back (translator workflow — for users editing JSON in their fork before submitting back). |

## Configuration

`glossa.config.json`:

```json
{
  "apiUrl": "https://glossa.example.com/api/v1",
  "apiKey": "glossa_...",
  "project": "brotwerk-site",
  "scan": {
    "include": ["src/**/*.{ts,tsx,vue,astro,html}"],
    "exclude": ["src/**/*.test.*"]
  },
  "pull": {
    "output": "./glossa/bundles"
  }
}
```

Include/exclude globs are composed with the repo's `.gitignore` via the [`ignore`](https://www.npmjs.com/package/ignore) package, so generated / vendored directories stay out without extra config.

## Extraction model

Regex-driven, not AST-driven. The patterns that matter (`key="..."` attributes, `t("...")` calls, string literals passed to known SDK methods) are stable enough that a regex pass is both fast and accurate. Each extracted key includes its source file + line number so missing-key diagnostics point straight at the call site.

## Honest scope

The CLI is build-time only. Runtime updates come via SSE in `<glossa-provider>` from `@felixgeelhaar/glossa-elements` — no polling, no CLI involvement.

## License

MIT
