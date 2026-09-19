# Glossa Studio

The web app for translators, developers and localization managers
(RFC 0002 §9): a Vue 3 + Vite SPA, TypeScript strict, zod at the API
boundary, on the Klarlabs design system. Everything it does goes through
the public `/v1` API (`platform/api/openapi.yaml`); nothing is private to
Studio.

**v0** is the foundation of the translator workspace (product intent §25,
§68): sign-in, projects, locales and the fallback graph, and a
keyboard-first editor with live MessageFormat 2 preview, structural QA
from the server, review and history. Releases is a placeholder until the
release endpoints land.

## Screens

| Route | What |
|---|---|
| `/auth/sign-in` | Magic link by default (the emailed link lands here as `#token=…`), passkey first when this browser enrolled one, password + TOTP under “Other ways”. |
| `/auth/register`, `/auth/reset-password` | Registration and password reset (link lands as `#token=…`). |
| `/t/:tenant` | Projects of the tenant; create one (source locale, authoring syntax, review requirement). The top bar switches tenants (personal and organizations) and creates organizations. |
| `…/p/:project/translate` | The translator workspace. Locale, filters and the selected key live in the query string, so a view can be bookmarked. |
| `…/p/:project/locales` | Locales with BCP 47 validation and direction; the list-based fallback graph editor. |
| `…/p/:project/settings` | Name, slug, syntax, review requirement, applications, delete. |
| `…/p/:project/releases` | Coming soon (see below). |
| `/account` | Passkeys, authenticator app (TOTP), sign out everywhere. After the first sign-in a banner promotes passkeys. |

The app shell carries the persistent **Public Beta** badge (Klarlabs
standard §5), a theme toggle (light/dark) and a skip link.

## Keyboard

Studio is keyboard-first. `?` shows this list in the app; it is rendered
from `src/lib/shortcuts.ts`, and a unit test keeps this table in step.

| Keys | Does | Where |
|---|---|---|
| `?` | Show keyboard shortcuts | Everywhere |
| `/` | Search messages | Translator workspace |
| `j` | Next message | Translator workspace |
| `k` | Previous message | Translator workspace |
| `Enter` | Edit the translation | Translator workspace |
| `Esc` | Leave the editor, back to the message list | Translation editor |
| `⌘ Enter` / `Ctrl Enter` | Save the translation | Translation editor |
| `⌘ ⇧ Enter` / `Ctrl Shift Enter` | Save and approve | Translation editor |
| `p` | Publish a release | Releases |

The message list is a WAI-ARIA listbox, so `↑`, `↓`, `Home`, `End`,
`PageUp` and `PageDown` work in it too. Single-key shortcuts never fire
while you type in a field, and `Enter` still activates a focused link or
button.

## The translator workspace

- **List**: virtualised (only visible rows are in the DOM), so thousands
  of messages scroll smoothly; pages load progressively. Filters:
  namespace, missing-in / outdated-in the target locale, message state
  (server-side) and search over key, source text and context (client-side).
  Rows show *Missing* / *Outdated* for the target locale.
- **Editor**: the developers' context, the source with argument chips from
  its derived metadata (type, selector kind and keys) and markup, the
  canonical MF2 on request, and the target editor with `lang`/`dir` of the
  target locale.
- **Live preview**: source and translation rendered with editable sample
  values, and one-click samples for every plural category of the *target*
  language (Arabic gets zero/one/two/few/many/other). It uses the reference
  formatter (`@glossa/messageformat`), loaded lazily. MF2 text is parsed as
  you type. **MF1 is parsed only by the server** — RFC 0002 §5 keeps exactly
  one MF1 converter, in Go — so an MF1 preview shows the last saved text
  and updates on save; the editor says so.
- **QA**: a write that fails structural QA (`422 structural_qa_failed`)
  shows its findings inline, marks the field invalid and keeps focus there;
  stored warnings show under the editor.
- **Outdated**: a badge with the source revisions involved and a word diff
  of the source since the translation was made.
- **Review**: approve, reject and request review, offered only when the
  member's roles and locale scope allow it (mirrored from Identity's role
  matrix; the server decides). `⌘⇧Enter` saves and approves in one step.
- **History**: every revision with kind, state, origin, `origin_detail`,
  author and the source revision it was made against.

## Development

```sh
pnpm install
pnpm --filter @glossa/messageformat build       # Studio imports its build
pnpm --filter @glossa/studio ui:link            # the design system, see below
# run glossa-server on :8080 (platform/README.md; GLOSSA_STUDIO_URL defaults to http://localhost:5173)
pnpm --filter @glossa/studio dev                # http://localhost:5173, /v1 proxied to GLOSSA_API_URL
```

Studio calls the API on its own origin. Vite (dev and preview) proxies
`/v1` to `GLOSSA_API_URL` (default `http://localhost:8080`); in production
the same host routes `/v1` to `glossa-server`. That keeps the
`__Host-glossa_session` cookie first-party and needs no CORS. Unsafe
requests carry `X-CSRF-Token` from the session.

| Script | Does |
|---|---|
| `dev`, `build`, `preview` | Vite (both `dev` and `build` provide the design system first). |
| `lint` | API types staleness check + `vue-tsc` (app, tests) + `tsc` (configs, e2e). |
| `test` | Vitest unit and component tests (happy-dom). |
| `test:e2e` | Playwright against a real server (below). |
| `gen:api` | Regenerate `src/api/schema.d.ts` from `platform/api/openapi.yaml`. |
| `size` | Report the initial JS bundle (gzip) from the build; fails over 250 kB. |

### API client

`src/api/schema.d.ts` is generated from the contract with
`openapi-typescript`; `openapi-fetch` is the typed client. `pnpm lint`
fails when the committed file is stale, so a contract change is picked
up in the same PR. Every response Studio trusts is validated with zod
(`src/api/schemas.ts`), and compile-time assertions tie each zod schema to
the generated type. Errors become `ApiError` with the problem's stable
`code` (never its title), field errors and QA findings.

### Design system (`@klarlabs-studio/ui`)

Studio uses the tokens (colours, type, spacing, motion via `--kl-*`),
`kl-badge` (the Public Beta badge) and `kl-theme-toggle`; its own
components consume the tokens. Product accent: see below.

The package is published to GitHub Packages, which needs a token even for
public packages, so an open-source checkout (and CI) can't install it from
there. Until it's on a public registry, Studio depends on it as
`"@klarlabs-studio/ui": "link:.klarlabs-ui"`, and
`scripts/klarlabs-ui.mjs` (`pnpm ui:link`, also run by `dev` and `build`)
makes `.klarlabs-ui` point at a built copy:

- by default it clones the public `klarlabs-studio/klarlabs` repository at
  the commit pinned in `package.json#klarlabsUi`, builds `packages/ui` once
  and links it (both gitignored);
- with `KLARLABS_UI_DIR=/path/to/klarlabs/packages/ui` it links that local,
  built checkout instead.

Type-checking and unit tests don't need it; `dev`, `build` and e2e do.

**Switching to the registry** when that's possible: set
`"@klarlabs-studio/ui": "0.1.0"`, add an `.npmrc` with
`@klarlabs-studio:registry=https://npm.pkg.github.com` and
`//npm.pkg.github.com/:_authToken=${NODE_AUTH_TOKEN}`, drop the `ui:link`
calls from `dev`/`build`, the `klarlabsUi` field and the CI step, and give
CI a token with `read:packages`.

### Accent

Glossa isn't in the standard's accent table (§6); Studio proposes
**Glossa — light and dark — `#2563EB` cobalt** (dark theme `#60A5FA`).
Reasons: no Klarlabs product uses blue in this band (Pet Medical's sky
`#0EA5E9` is a lighter cyan); and a translation tool leans on semantic
colours all day — green approved, amber needs review and warnings, red
rejected and QA errors — so a teal or amber accent would read as a review
state. `#2563EB` passes AA as text on white and under white text; text on
accent tints uses the deeper `#1D4ED8` (`--gs-accent-ink`). The overrides
are in `src/styles/studio.css`.

## Tests

- **Unit and component** (`pnpm test`): BCP 47 checks, fallback-graph
  validation (mirrors the server's cycle rules), permissions, diff, samples
  and plural categories, virtualisation, shortcuts, WebAuthn encoding, the
  API boundary (CSRF, ETags, problem codes, zod), and the list, preview,
  editor, locale input and releases components.
- **End to end** (`pnpm test:e2e`, `e2e/`): the global setup starts
  Postgres 16 with testcontainers, provisions it like production (a
  `CREATEROLE` owner migrates, the server runs as `glossa_app`), builds and
  starts the real `glossa-server` from `../platform` with the log mailer, and
  Playwright serves the Studio build with `vite preview`. The main test signs
  in with the magic link the dev mailer captured, creates a project, adds
  locales, imports a small catalog through the API, translates with the
  keyboard, hits a QA error for a broken placeholder, fixes it, approves and
  checks history, the shortcut sheet and the Releases placeholder.
  `@axe-core/playwright` (WCAG 2.2 AA tags) runs on each main screen, light
  and dark.

  Needs Docker, Go and `pnpm build` first:

  ```sh
  pnpm --filter @glossa/studio build
  pnpm --filter @glossa/studio exec playwright install chromium
  pnpm --filter @glossa/studio test:e2e
  ```

  Ports: `GLOSSA_E2E_API_PORT` (18317), `GLOSSA_E2E_STUDIO_PORT` (4317).
  `GLOSSA_E2E_SERVER_BIN` uses a prebuilt server. `STUDIO_SCREENSHOTS=<dir>`
  keeps a screenshot of every screen axe checked. CI runs it as the
  `Studio e2e` job.

## Releases

`src/api/releases.ts` defines `ReleasesPort` (`availability`, `list`,
`publish`), and `main.ts` provides the coming-soon adapter. When the
`/v1` release endpoints are in the contract: run `gen:api`, add zod
schemas for the release resources, implement the port over the generated
client, and provide it instead. `ReleasesView` already renders the
available state (list, publish), which its component test covers with a
fake port.

## Layout

```text
src/api/        generated contract types, client, zod schemas, endpoints, errors, releases port
src/session/    session store, permission mirror
src/lib/        pure logic: bcp47, fallback, diff, samples, preview, shortcuts, virtual, webauthn, …
src/components/ app shell, message list, editor, preview, QA, history, …
src/views/      auth, projects, account, project/* (workspace, locales, settings, releases)
src/strings.ts  every user-facing string, ready to become Glossa messages
e2e/            Playwright specs and the server harness
```
