# glossa

The Glossa CLI: one static binary for developers and CI (product intent
§46, RFC 0002 §9). It talks to `glossa-server` through the `/v1` API with
an API token, and runs the MessageFormat kernel locally, so `check
--offline`, `extract` and `generate` work without a server.

```sh
go build -o glossa ./cmd/glossa        # from platform/
glossa init --server https://glossa.example.com --project brotwerk
glossa login                            # or GLOSSA_TOKEN=glossa_api_… in CI
glossa push && glossa check && glossa generate
```

Release binaries (darwin/linux/windows × amd64/arm64, `CGO_ENABLED=0`):
`goreleaser build --snapshot --clean --config cmd/glossa/.goreleaser.yaml`
from `platform/`. Nothing is published.

## glossa.yaml

Found by walking up from the working directory. Paths are relative to
the file. `GLOSSA_SERVER`, `GLOSSA_TENANT` and `GLOSSA_PROJECT` override
it.

```yaml
version: 1
server: https://glossa.example.com
tenant: 0192…              # optional: the API token's own tenant
project: brotwerk          # slug or ID
source_locale: de          # init reads it from the server
syntax: mf1                # local catalogs: mf1 (ICU, default) or mf2
catalogs:
  path: locales/{locale}.json   # the source file is pushed; the others hold translations
  style: flat                   # how pull writes: flat (default) or nested
extract:
  include: ["**/*.{ts,tsx,js,jsx,mjs,vue,astro,svelte,html,go}"]
  exclude: ["**/*.test.*", "**/*_test.go", "**/*.d.ts"]
generate:
  typescript: src/glossa/messages.ts
  vue: src/glossa/glossa-vue.ts  # needs typescript
  go: internal/msg/messages.go
  go_package: msg                # default: the directory name
check:
  require_complete: [de, en]     # default: every locale
  fail_on: error                 # or warning
pull:
  states: [approved]             # default
```

Catalogs are JSON objects of message ID to text, flat
(`{"checkout.pay": "…"}`) or nested (`{"checkout": {"pay": "…"}}`).

## Authentication

`GLOSSA_TOKEN` wins (CI). Otherwise `glossa login` verifies a token
against the server and stores it per server URL in the macOS Keychain or
the Secret Service keyring (Linux, via `secret-tool`), falling back to a
0600 `glossa/credentials.json` under the user config directory. Tokens
reach the keychain tools on stdin, never on the command line.
`glossa logout` removes it; `glossa whoami` shows what's in use.

## Commands

Every command takes `--json`, `--quiet`, `--no-color` and `--config`.
Colors appear only on a terminal (and never with `NO_COLOR`).

| Command | What it does |
|---|---|
| `init` | Writes glossa.yaml. Prompts on a terminal; with a token, reads the tenant and source locale from the server. `--server --project --source-locale --catalogs --typescript --vue --go --force` |
| `login` / `logout` / `whoami` | Token storage; `--server`, `--token-stdin` |
| `push` | Sends the source catalog through `message-upserts` (500 per request). Reports created/revised/updated/unchanged/failed per key. `--dry-run` compares canonical models with the server instead of writing. `--translations` also imports the other catalogs as translations (provenance `import`). |
| `pull` | Writes translations to the catalogs, sorted and deterministic. `--states approved,needs_review\|all`, `--locales`. `--release <id\|v<N>\|latest> [--environment env] [--out dir]` writes a release bundle instead (see *Release*). |
| `extract` | Finds usages: `<glossa-text key\|message\|id="…">`, `<GlossaText id="…">`, `t("…")`, `$t("…")`, Go `x.T(ctx, "…")` / `l.T("…")`, `{{t "…"}}`, and the generated accessors. Reports file:line, IDs missing from the catalog, and catalog messages nothing uses. `--strict` exits 1 on unknown IDs. |
| `generate` | Typed accessors from the catalog's argument metadata. `--check` writes nothing and exits 1 when the files are stale; `--from-server` uses the server's messages. |
| `check` | Structural QA: invalid messages, translation/source compatibility (`messageformat.CheckCompat`), missing and outdated translations. `--offline` checks local catalogs. `--require-complete=de,en\|none`, `--fail-on=error\|warning`. |
| `status` | Coverage per locale: translated, approved, needs review, draft, outdated, missing. One request: the server's `translation-stats`. `--offline` counts the local catalogs. |
| `diff` | Local catalogs vs the server by canonical model (so MF1 spelling changes aren't changes). `--exit-code`. |
| `locales`, `messages` | Lists. `messages --prefix --namespace --missing-in --outdated-in --state active\|obsolete\|all` |
| `import --from v0` | Imports a Glossa v0.3 project (below). |
| `release` | `publish [--dry-run]`, `list`, `show`, `diff`, `promote`, `rollback`, `environments`, `keys [list\|create\|revoke]` (see *Release*). |

`check` reads like CI output:

```text
✓ 42 messages discovered (brotwerk on https://glossa.example.com)
✓ message structures valid
✓ arguments valid
✓ de complete
✗ en 1 missing
  error checkout.payment_failed  missing translation

Localization check failed: 1 error, 0 warnings.
```

Missing translations are errors in required locales and warnings in the
others; outdated translations and compatibility warnings are warnings.
Checks are `qa.Checker`s, so source-copy lint (M3) and later layers plug
in without changing the command.

## Exit codes

| Code | Meaning |
|---|---|
| 0 | OK |
| 1 | A check failed: `check`, `diff --exit-code`, `generate --check`, `extract --strict`, `release publish --dry-run` (not releasable) |
| 2 | Usage or configuration: bad flags, missing/invalid glossa.yaml or catalog, unavailable command, input the server rejects as invalid (`invalid_environment`, `invalid_note`, `invalid_key_name`, `idempotency_key_reused`) |
| 3 | Network or auth: server unreachable, token missing or refused, forbidden, not found, server error, or the server refusing the operation (`release_ineligible`, `no_rollback_target`, `not_in_history`, `not_releasable`, `key_revoked`, `storage_unavailable`) |
| 4 | Partial failure: `push` or `import` went through but some items failed |

Errors print what happened, where, why and how to fix it:

```text
error: no API token for https://glossa.example.com
  where: https://glossa.example.com
  why:   GLOSSA_TOKEN is unset and `glossa login` stored no token for this server
  fix:   run `glossa login`, or set GLOSSA_TOKEN (CI) to a token created in Studio
```

## JSON output

With `--json` a run prints exactly one JSON document to stdout, tagged
with `schema`. New fields may be added; existing ones keep their meaning.

| Schema | Shape |
|---|---|
| `glossa.cli.error/v1` | `{error: {code, exit_code, message, where?, why?, fix?}}` |
| `glossa.cli.init/v1` | `{path, config, checked_with_server}` |
| `glossa.cli.login/v1` | `{server, stored_in, tenant: {id, slug, name, kind}}` |
| `glossa.cli.whoami/v1` | `{server, token (redacted), token_source, tenant, project?: {id, slug, name, source_locale}}` |
| `glossa.cli.push/v1` | `{dry_run, source, summary: {created, revised, updated, unchanged, failed}, messages: [{key, status, revision?, error?: {code, detail}}], translations?: [{key, locale, status, state?, error?}]}` |
| `glossa.cli.pull/v1` | `{states, locales: [{locale, path, messages, skipped: {state: n}, outdated, changed}], release?: {dir, release_id, version, environment, locales, artifacts, bytes, removed}}` |
| `glossa.cli.extract/v1` | `{files, usages: [{key, file, line, column, kind}], unknown: [{key, locations: [{file, line}]}], unused: [key]}` |
| `glossa.cli.generate/v1` | `{source, messages, check, files: [{path, kind, changed}], warnings: [{key, reason}]}` |
| `glossa.cli.check/v1` | `{policy: {require_complete (null = all), fail_on}, origin, messages, invalid_messages, locales: [{code, is_source, required, messages, translated, missing, outdated, errors, warnings, complete}], findings: [{check, code, severity, locale?, key?, subject?, detail?, message, where?}], errors, warnings, passed}` |
| `glossa.cli.status/v1` | `{origin, messages, locales: [{code, direction, is_source, translated, approved, needs_review, draft, rejected, outdated, missing, coverage}]}` |
| `glossa.cli.diff/v1` | `{source: {locale, added, changed: [{key, local, server}], removed, unchanged}, translations: [same], identical}` |
| `glossa.cli.locales/v1` | `{locales: [{code, direction, is_source}], fallback}` |
| `glossa.cli.messages/v1` | `{messages: [{key, namespace, state, source_revision, text, syntax, arguments: [{name, type}], description?}]}` |
| `glossa.cli.import/v1` | `{from, source: {url, project}, dry_run, locales_added, summary: {message: {status: n}, translation: {status: n}}, items: [{kind, key, locale, status, v0_status?, state?, downgraded?, reason?, error?}]}` |
| `glossa.cli.release.publish/v1` | `{replayed, idempotency_key, release: Release}` |
| `glossa.cli.release.preview/v1` | `{environment, policy: {states, include_outdated}, base: Ref \| null, releasable, problems: [{code, detail, key?, locale?}], release: {source_locale, locales: [code], manifest_digest, counts} \| null, changes: [{locale, added, changed, removed}], identical}` (`release publish --dry-run`) |
| `glossa.cli.release.list/v1` | `{releases: [Release & {serving: [environment]}]}` (newest first) |
| `glossa.cli.release.show/v1` | `{release: Release, serving: [environment]}` |
| `glossa.cli.release.diff/v1` | `{release: Ref, base: Ref \| null, locales: [{locale, added, changed, removed}], identical}` |
| `glossa.cli.release.promote/v1`, `glossa.cli.release.rollback/v1` | `{environment: Environment, previous: Ref \| null}` |
| `glossa.cli.release.environments/v1` | `{environments: [Environment]}` |
| `glossa.cli.release.keys/v1` | `{keys: [DeliveryKey]}` |
| `glossa.cli.release.key/v1` | `{action: created \| revoked, key: DeliveryKey}` |

The release shapes share:

- `Release`: `{id, version, environment (published to), parent_id?, note?, author, created_at, source_locale, locales: [code], manifest_digest, policy: {states, include_outdated}, counts: {messages, artifacts, new_artifacts, bytes, locales: {code: {messages, outdated}}}}`
- `Ref`: `{id, version}`
- `Environment`: `{name, release: Ref | null, policy: {states, include_outdated}, updated_at}`
- `DeliveryKey`: `{id, name, key, created_at, revoked_at?}`

Finding codes are the kernel's (`missing-argument`, `extra-argument`,
`argument-type-changed`, `selector-*`, `invalid-plural-key`,
`missing-plural-category`, `markup-*`, plus the server's
`max-length-exceeded`) and the CLI's: `invalid-message`,
`invalid-translation`, `missing-translation`, `outdated-translation`,
`unknown-key`, `missing-locale`.

## Typed messages

`generate` writes, from `messageformat.Arguments`:

```ts
// messages.ts
export interface Messages {
  "cart.items": { count: number };
  "checkout.pay": { amount: number };
  "athlete.greeting": { gender: "female" | (string & {}); name: string };
}
export function createMessages(t: Translate) { … }   // messages.checkout.pay({ amount })

// glossa-vue.ts
declare module "@glossa/vue" { interface GlossaRegister { messages: Messages } }
export function useTypedMessages() { … }              // in setup(): m.cart.items({ count })
```

```go
// messages.go
m := msg.For(client.For("de"))          // or msg.FromContext(ctx, client)
subject := m.CheckoutPay(order.Total)   // func (m Messages) CheckoutPay(amount float64, opts ...glossa.Option) string
```

Types: numbers, integers, percents, currencies and units are `number` /
`float64` (`int` for integers); dates and times `Date | number | string` /
`time.Time`; selects the literal cases plus any string; everything else a
string. Key segments become camelCase (TS) and PascalCase (Go); keys whose
accessor would collide keep working by ID and are reported as warnings.

## Import from Glossa v0.3

```sh
export GLOSSA_V0_KEY=glossa_…   # a v0.3 project key (read is enough)
glossa import --from v0 --v0-url https://old.example.com/api/v1 --v0-project brotwerk-site --dry-run
glossa import --from v0 --v0-url https://old.example.com/api/v1 --v0-project brotwerk-site
```

- Reads `GET /projects/{slug}/locales` and each locale's
  `GET /projects/{slug}/locales/{locale}/messages` (`{messages, statuses}`).
- The project's source locale must exist in v0.3. Its values become
  messages (ICU MF1, parsed and validated by the kernel for the source
  locale); empty values are skipped.
- Missing locales are added. Every other non-empty value becomes a
  translation with provenance `import` and `origin_detail`
  `{source: "glossa-v0.3", url, project, status}`.
- Statuses: `approved` → `approved`, `needs_review` and `ai_translated`
  → `needs_review`, `pending` → `draft`. Where the project requires
  review, API tokens can't approve, so approved values arrive as
  `needs_review` and are reported `downgraded`.
- Idempotent: messages are upserted; translations the server already
  has (same canonical model) aren't sent again, so a re-run never undoes a
  review made in between. Every key is reported.

## Release

A release is an immutable build of the project's active messages and
the translations an environment's policy ships (production and staging:
`approved`; development and preview: everything not rejected).
Environments point at releases; glossa-edge serves what they point at.
The token needs the `publish` scope to publish, promote, roll back and
manage keys; `read` is enough for the rest. Releases are named by ID or
`v<N>`.

```sh
glossa release publish --environment staging --dry-run   # what would ship, what would change; stores nothing
glossa release publish --environment staging --note "sprint 42"
glossa release promote v7 --to production      # moves the pointer, rebuilds nothing
glossa release rollback --environment production [--to v6]
glossa release diff v6 v7                      # or `diff v7`: against its parent
glossa release list | show v7 | environments
glossa release keys create web                 # keys | keys revoke web
glossa pull --release latest --environment production --out public/glossa
```

```text
✓ Published v7 to staging (0192…)
  42 messages · de 42, en 41 (1 outdated) · 2 artifacts (1 new), 12.4 KiB
  note: sprint 42

✓ production now serves v7 (0192…); was v6

v6 → v7 (0191… → 0192…)
  de  1 added, 0 changed, 0 removed
    + checkout.payment_failed
  en  unchanged
```

- The path to production: `development` and `preview` ship work in
  progress (everything not rejected); `staging` and `production` ship
  approved text only. Publish to `staging`, check it, then promote that
  release to `production`. A development or preview release can't be
  promoted to production; the refusal (`release_ineligible`) names both
  policies and what differs.
- `publish --dry-run` runs the server's build for the environment and
  stores nothing (no release, artifacts or events; `read` scope is
  enough): per-locale counts, the artifacts a publish would upload, and
  the message IDs each locale would add, change or remove against what
  the environment serves. A catalog that can't be released lists every
  problem and exits 1. `--note` and `--idempotency-key` have no effect
  with it.
- `publish` defaults to `development`, so nothing reaches production by
  accident. Every invocation sends a new `Idempotency-Key` (a CI retry of
  the whole step publishes again; retries inside the run replay). Pass
  `--idempotency-key` (e.g. the CI run ID) to make the step itself
  repeatable: a replay reports `replayed: true` and publishes nothing,
  and the same key with a different request fails with
  `idempotency_key_reused`.
- `promote` needs the environment's policy to cover the release's
  (`release_ineligible` otherwise: a preview release with drafts can't
  reach production). `rollback` without `--to` goes to the newest
  release the environment served before its current one, so running it
  twice goes two steps back.
- `environments` and `list` say which release each environment serves.
- `keys create <name>` prints the delivery key runtimes fetch releases
  with (`/v1/<key>/<environment>/manifest.json` on glossa-edge). Keys are
  publishable by design (they ship in browser bundles), scoped to the
  project and read-only. `keys revoke` takes the ID or the name of an
  active key.
- `pull --release` writes the bundle runtimes load offline
  (runtimes/SPEC.md §3.4): `manifest.json` exactly as the edge serves it
  for `--environment`, and `a/<sha256>.json` for every artifact, each
  checked against its hash, the manifest last; artifacts of an earlier
  bundle it no longer names are removed. Runtimes reject a manifest for
  another environment than theirs, so the bundle is written for one:
  `latest` is what `--environment` serves now; a release ID or `v<N>`
  defaults to the environment it was published to.

## Known limits

- `check`, `diff`, `pull`, `push --translations` and `import` read
  translations with the project-wide listing (`GET …/translations`,
  active messages, 100 a page, 20 locales a request) and join them to
  the catalog by message ID; `status` reads `GET …/translation-stats`.
  `check` needs every translation's content for its QA, so the stats
  (counts only) don't replace the listing there. The listing reads
  Localization's view of the catalog, current once Catalog's events are
  processed (usually within a second): a message reactivated a moment
  ago can briefly show as missing.
- `extract` is lexical: it finds literal IDs, not computed ones, and
  doesn't read `.gitignore` (it skips hidden directories, `node_modules`,
  `dist`, `vendor`, `build` and `coverage`).
- Windows stores tokens in the file store.
