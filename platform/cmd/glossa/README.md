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
| `tm` | Translation memory: `search <text> --to L`, `concordance <text>`, `units [--locale-pair de:en] [--retire <id>]` (see *Knowledge and AI*). |
| `terms` | Termbase: `list`, `show`, `add`, `edit`, `deprecate`, `forbid`, and `check`, terminology QA over the project's translations. |
| `style` | `show [--locale --namespace]`: the effective style guide; `edit --file style.yaml`: create or replace one. |
| `translate` | `--locale L [--namespace --key-prefix] [--missing\|--outdated] [--dry-run] [--wait]`: fills locales with AI suggestions. |
| `review` | The AI review queue: `list [--locale]`, `accept <id\|key> [--text]`, `reject <id\|key> [--reason]`. |
| `ai status` | Provider consent, the monthly budget and spend, providers (never their keys), and the project's auto-translate locales, namespace tags and review routing. |

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
in without changing the command. `check --terminology` adds the
termbase layer (check `terminology`: the server's terminology QA over
every translation but rejected ones): a forbidden term is an error, a
deprecated or missing one a warning.

## Exit codes

| Code | Meaning |
|---|---|
| 0 | OK |
| 1 | A check failed: `check`, `terms check`, `diff --exit-code`, `generate --check`, `extract --strict`, `release publish --dry-run` (not releasable), `translate --dry-run` (a refusal: consent off, no budget, no provider) |
| 2 | Usage or configuration: bad flags, missing/invalid glossa.yaml, catalog or style file, unavailable command, input the server rejects as invalid (`invalid_environment`, `invalid_note`, `invalid_key_name`, `idempotency_key_reused`, and every 400 of the Knowledge and Intelligence APIs, e.g. `invalid_locale`, `duplicate_term`), `locale_not_found`, an ambiguous term or key (`term_ambiguous`, `suggestion_ambiguous`) |
| 3 | Network or auth: server unreachable, token missing or refused, forbidden, not found (`term_not_found`, `suggestion_not_found`), server error, or the server refusing the operation (`release_ineligible`, `no_rollback_target`, `not_in_history`, `not_releasable`, `key_revoked`, `storage_unavailable`, `suggestion_decided`, `suggestion_outdated`, `translation_conflict`, `translation_rejected`, `precondition_failed`), `translate --wait` giving up (`wait_timeout`) |
| 4 | Partial failure: `push` or `import` went through but some items failed; `translate --wait`: some jobs failed |

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

| `glossa.cli.tm.search/v1` | `{query: {text, from, to, syntax}, source_normalized, matches: [{score, kind (exact \| context \| fuzzy), target (MF2), target_text, target_syntax (the query's syntax; mf2 when MF1 can't express the target), variables_adapted, unit: Unit}]}` |
| `glossa.cli.tm.concordance/v1` | `{query: {text, side, from?, to?}, matches: [{similarity, unit: Unit}]}` |
| `glossa.cli.tm.units/v1` | `{units: [Unit]}` |
| `glossa.cli.tm.retire/v1` | `{unit: Unit}` |
| `glossa.cli.terms.list/v1` | `{concepts: [Concept]}` |
| `glossa.cli.terms.show/v1` | `{concept: Concept}` |
| `glossa.cli.terms.change/v1` | `{action: created \| updated \| unchanged \| deprecated \| forbidden, concept: Concept}` (`add`, `edit`, `deprecate`, `forbid`) |
| `glossa.cli.terms.check/v1` | `{fail_on, locales: [{code, checked, errors, warnings}], findings: [{code (term_forbidden \| term_missing), severity, locale, key, side (source \| target), text, start, end, suggestions, concept_id, term_id, message}], checked, errors, warnings, passed}` |
| `glossa.cli.style.show/v1` | `{scope: Scope, fields: StyleFields, rules: [StyleRule], sources: [{style_guide_id, version, project_id?, locale?, namespace?}]}` |
| `glossa.cli.style.edit/v1` | `{action: created \| updated \| unchanged, guide: {id, name, scope: Scope, version, fields: StyleFields, rules: [StyleRule]}}` |
| `glossa.cli.translate/v1` | `{dry_run, locales, filter: {namespace?, key_prefix?, missing, outdated}, plan: [{locale, queue, keys, existing, tm_exact, provider, refused: {reason: n}, cost: Cost}] (dry run), skipped: {reason: n}, refusals: [{code, message, fix}], cost?: Cost (dry run), fills: [{id, locales, keys?, jobs_created, jobs_existing, skipped, job_states, warnings}], wait: {elapsed_ms, job_states: {state: n}, failed: [{id, key, locale, state, failure_code?, error?}]} \| null}`; `Cost` is `{estimated_micro_usd, max_micro_usd, unpriced}` |
| `glossa.cli.review.list/v1` | `{suggestions: [Suggestion]}` (riskiest first) |
| `glossa.cli.review.decision/v1` | `{decision: accepted \| rejected, edited, suggestion: Suggestion}` |
| `glossa.cli.ai.status/v1` | `{consent: {enabled, changed_at?, changed_by?}, max_concurrent_jobs, budget: {monthly_micro_usd, spent_micro_usd, remaining_micro_usd, month_start, calls, by_provider: [{provider, model, calls, cost_micro_usd, input_tokens, output_tokens}]}, providers: [{name, kind, enabled, api_key_set, base_url?, models}], project: {auto_translate_locales, namespace_tags: {namespace: [tag]}, review: {auto_approve, auto_approve_min, recommend_min, auto_approve_environments?, force_review?}}}` |

The Knowledge and AI shapes share:

- `Unit`: `{id, source_locale, target_locale, source, target, project_id (null: tenant-wide), message_key?, namespace?, origin, state (active \| retired), hit_count, created_at, retired_at?, retired_reason?}`
- `Concept`: `{id, project_id (null: tenant-wide), definition, domain, note, version, terms: [{id, locale, text, status (preferred \| admitted \| deprecated \| forbidden), part_of_speech?, case_sensitive, note?}], updated_at}`
- `Scope`: `{project_id, locale, namespace}` (null: broader)
- `StyleFields`, `StyleRule`: the API's (`formality: {register, pronoun}`, `tone`, `punctuation`, `numbers`, `dates`; `{id, title?, rationale?, good?, bad?, disabled?}`)
- `Suggestion`: `{id, key, locale, namespace, text (MF2), score, action (auto_approve \| approve_recommended \| review_required), action_note?, risk_tags, status, origin (ai \| translation_memory), provider?, model?, explanation: [{factor, value, contribution, reason}], findings: [{code, severity?, message, term?}], translation_revision?, created_at}`

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

## Knowledge and AI

The termbase, translation memory and style guides (RFC 0003 §2) and AI
translation (§3). The token needs `read` to search, list and check, and
`write` to change the termbase and style guides, retire units, run
`translate` and decide suggestions. Approving translations stays with
people (Studio), and so do provider keys, consent and budgets.

```sh
glossa terms add Warenkorb --preferred en=cart --forbidden en=basket --definition "Where items wait"
glossa terms forbid trolley --locale en --concept <id>
glossa terms check --locale en            # CI: exit 1 on a forbidden term
glossa check --terminology                # structural QA plus the termbase
glossa tm search "Bezahle {amount, number}" --to en
glossa style edit --file style/de.yaml --locale de && glossa style show --locale de
glossa translate --locale ja --dry-run    # what would be queued, and why it would do little
glossa translate --locale ja --wait       # queue, then poll until the jobs finish
glossa review list --locale ja && glossa review accept checkout.pay --locale ja
```

```text
✓ 12 translations checked against the termbase (brotwerk on https://glossa.example.com)
✗ en 1 error, 0 warnings
  error cart.items  term_forbidden: "loaf" is forbidden (use "bread")
✓ ja no findings

Terminology check failed: 1 error, 0 warnings.
```

- `tm search` parses the text in glossa.yaml's `syntax` (`--syntax`)
  and matches it against the project's and the tenant-wide units
  (`--all-projects`: every project's). Targets come back renamed to the
  query's variables, in MF2 (`target`) and in the query's syntax
  (`target_text`, shown in the table; MF2 when MF1 can't express the
  target). `tm units` lists the project's units;
  `--retire <id>` takes one out of matching.
- `terms add <term>` makes it preferred in `--locale` (default: the
  source locale); `--preferred|--admitted|--deprecated|--forbidden
  L=TEXT` add more. Concepts belong to the project unless
  `--tenant-wide`. `show`, `deprecate` and `forbid` find a concept by a
  term's text (`--locale` narrows it) or take its ID; `--concept` names
  it, and lets `forbid` add a term the concept lacks. Changes replace the
  concept conditionally (`If-Match`), so concurrent edits fail instead of
  overwriting each other.
- `terms check` has the server check every translation but rejected
  ones (`--states` narrows it) in every target locale (`--locale`,
  repeatable) — `GET …/terminology-findings`, a page of 100 translations
  a request, 20 locales at a time — and fails on errors (`--fail-on
  warning`, or glossa.yaml's `check.fail_on`). `check --terminology`
  adds the same findings to structural QA.
- `style edit --file` takes YAML with `name`, `fields` and `rules` (the
  API's field names; an unknown field is an error) and creates or
  replaces the guide of exactly the scope given: the project (default) or
  the tenant (`--tenant-wide`), plus `--locale` and `--namespace`. `style
  show` merges every applicable guide, the narrowest winning, and names
  the versions it used.
- `translate` queues one job per message missing in each locale
  (`--outdated`: outdated ones instead; both flags: both) in one fill
  that selects them by state on the server (`select`), never for
  sensitive namespaces. `--dry-run` queues nothing: the server's fill
  preview (`ai-fill-previews`) lists the keys per locale and how each
  would run — `existing` jobs reused, `tm_exact` (an exact
  translation-memory match, no provider call), `provider`, or `refused`
  by reason (`provider_consent`, `no_route`, `budget_exceeded`;
  sensitive messages count as `skipped.sensitive`) — with the estimated
  and upper-bound cost of the provider calls, and the refusals the fill
  would warn about (`provider_consent_off`, `no_budget`,
  `budget_exhausted`, `no_provider`); it exits 1 when there is one. A
  push is visible to it (and to `translate`) as soon as `push` returns.
  `--wait` prints progress
  to stderr and exits 4 when a job failed, each with its `failure_code`
  (`provider_consent`, `invalid_output`, `budget_exceeded`, `no_route`,
  …). Each invocation sends a new `Idempotency-Key`.
- `review accept` writes the suggestion as the translation, in the state
  the project's review policy gives an API token (tokens never approve).
  `--text` accepts your edit instead, in glossa.yaml's syntax
  (`--syntax`). A key names that message's pending suggestion (with
  `--locale` when it has several).

## Known limits

- `check`, `diff`, `pull`, `push --translations` and `import` read
  translations with the project-wide listing (`GET …/translations`,
  active messages, 100 a page, 20 locales a request) and join them to
  the catalog by message ID; `status` reads `GET …/translation-stats`.
  `check` needs every translation's content for its QA, so the stats
  (counts only) don't replace the listing there. The listing reads
  Localization's view of the catalog: current when a push (`message-upserts`)
  returns; after other message writes (Studio edits, renames) once their
  events are processed, usually within a second.
- `extract` is lexical: it finds literal IDs, not computed ones, and
  doesn't read `.gitignore` (it skips hidden directories, `node_modules`,
  `dist`, `vendor`, `build` and `coverage`).
- Windows stores tokens in the file store.
- `translate --dry-run`'s `tm_exact` counts exact translation-memory
  matches; the job still validates one before reusing it (structure,
  plural categories, terminology), so a match that fails there goes to a
  provider after all. Its cost is an estimate from the price table, not
  a quote.
