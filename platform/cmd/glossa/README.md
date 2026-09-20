# glossa

The Glossa CLI: one static binary for developers and CI (product intent
§46, RFC 0002 §9). It talks to `glossa-server` through the `/v1` API with
an API token, and runs the MessageFormat kernel locally, so `check
--offline`, `extract` and `generate` work without a server.

```sh
go build -o glossa ./cmd/glossa        # from platform/
glossa init --server https://glossa.example.com --project brotwerk
glossa login                            # in GitHub Actions: no secret at all
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
  templates: ["**/*.{tmpl,gotmpl,gohtml}"]  # read as Go templates
  application: web               # the usages document's application
generate:
  typescript: src/glossa/messages.ts
  vue: src/glossa/glossa-vue.ts  # needs typescript
  react: src/glossa/glossa-react.ts  # needs typescript
  go: internal/msg/messages.go
  go_package: msg                # default: the directory name
check:
  require_complete: [de, en]     # default: every locale
  fail_on: error                 # or warning
pull:
  states: [approved]             # default
capture:                         # glossa capture: screenshots with message regions
  base_url: http://localhost:4173  # a preview with fixture data; --base-url overrides
  application: web               # default: extract.application
  output: .glossa/captures       # without --upload (default)
  locales: [de, ja]              # default: the source locale
  locale: { query: lang }        # how a page picks the locale: query or cookie,
                                 # or put {locale} in every route's url
  viewports:                     # default: 1280×800 and 390×844
    - { width: 1280, height: 800 }
    - { width: 390, height: 844, device_scale_factor: 2, mobile: true }
  routes:
    - route: /                   # the pattern usages name; url defaults to it
    - route: /checkout/[step]
      url: /checkout/payment     # relative to base_url, or absolute
      playbook: .glossa/playbooks/open-card.json  # scout playbook, replayed after load
  cookies:                       # fixture login, set on base_url
    - { name: session, value: "${FIXTURE_SESSION}" }
  headers:                       # sent only to base_url's origin
    X-Fixture-User: "${FIXTURE_USER}"
```

Cookie and header values reference environment variables as `${NAME}`;
they're never printed, and an unset one is an error that names only the
variable.

Catalogs are JSON objects of message ID to text, flat
(`{"checkout.pay": "…"}`) or nested (`{"checkout": {"pay": "…"}}`).

## Authentication

`GLOSSA_TOKEN` wins. Otherwise `glossa login` verifies a token against
the server and stores it per server URL in the macOS Keychain or the
Secret Service keyring (Linux, via `secret-tool`), falling back to a
0600 `glossa/credentials.json` under the user config directory. Tokens
reach the keychain tools on stdin, never on the command line.
`glossa logout` removes it; `glossa whoami` shows what's in use.

### In GitHub Actions: no stored secret

With `permissions: { id-token: write }`, `glossa login` needs no token
at all (RFC 0004 §6.3). It sees `ACTIONS_ID_TOKEN_REQUEST_URL` and its
request token, asks GitHub for an OIDC ID token with audience `glossa`,
and exchanges it at `POST /v1/auth/github-oidc-exchanges`.

```yaml
permissions:
  contents: read
  id-token: write

steps:
  - uses: ./.github/actions/glossa      # or the commands by hand:
  - run: glossa login                   # no secret, nothing to rotate
  - run: glossa push --branch "$GITHUB_HEAD_REF" --pr "$PR_NUMBER"
```

The server verifies the issuer, the audience, the signature and the
expiry, then matches the token's `repository_id` — GitHub's immutable
number, never the repository's name — against a **Git connection**
(*Git connections*, below). What it hands back:

- lives **30 minutes** and cannot be refreshed: the next job asks GitHub
  for a fresh ID token;
- is bound to **one project**, so a request under another project of the
  same workspace is refused;
- allows exactly `catalog.read` and `catalog.write` — what `push`,
  `extract --upload`, `context push`, `capture --upload` and
  `preview register` need. It cannot import translations
  (`push --translations`), publish a release, use the termbase or AI, or
  read the workspace's members or tokens. Those still need an API token.

`glossa login --json` says which project it authenticated for, what the
credential may do and when it dies; the credential itself is stored the
same way as any other, and never printed.

Refusals, and what they mean:

| Code | What to do |
|---|---|
| `repository_not_connected` | Connect the repository to a project in Studio → Settings → GitHub. A project the repository does not feed gets the same answer. |
| `ambiguous_project` | The repository feeds several projects (a monorepo, one connection per path). `login` prints their IDs; pass `--project <id>`. |
| `invalid_id_token` | The ID token didn't verify: check the audience is `glossa`, and the server's clock and GitHub configuration. |
| `github_not_configured` | The server has no GitHub App, so there are no connections to match against. Use an API token. |

**Forks.** A pull request from a fork gets no ID token and no secrets
from GitHub, so a fork's job cannot authenticate and Glossa's check on
it completes as `neutral` with an explanation (RFC 0004 §6.4).

**Keeping a token instead.** `--token-stdin` means an API token even
inside Actions, and `GLOSSA_TOKEN` wins over everything, so a workflow
that already keeps a secret goes on working unchanged. On CI that isn't
GitHub Actions, that is the way: set `GLOSSA_TOKEN` and run the same
commands (see `.github/actions/glossa/README.md`).

A self-hosted deployment, or GitHub Enterprise Server, sets the issuer
and audience it accepts with `GLOSSA_GITHUB_OIDC_ISSUER`,
`GLOSSA_GITHUB_OIDC_AUDIENCE` and `GLOSSA_GITHUB_OIDC_MAX_STALE`.

## Commands

Every command takes `--json`, `--quiet`, `--no-color` and `--config`.
Colors appear only on a terminal (and never with `NO_COLOR`).

| Command | What it does |
|---|---|
| `init` | Writes glossa.yaml. Prompts on a terminal; with a token, reads the tenant and source locale from the server. `--server --project --source-locale --catalogs --typescript --vue --react --go --force` |
| `login` / `logout` / `whoami` | Credential storage; `--server`, `--token-stdin`, `--project` (GitHub Actions OIDC; see *Authentication*) |
| `push` | Sends the source catalog through `message-upserts` (500 per request). Reports created/revised/updated/unchanged/failed per key. `--dry-run` compares canonical models with the server instead of writing. `--translations` also imports the other catalogs as translations (provenance `import`). `--branch <name> [--pr <n>] [--commit <sha>]` pushes as a feature branch instead (RFC 0004 §4.1): the whole catalog in one request, where a new key becomes a `proposed` message the branch owns, changed source for a live key a source proposal, and nothing live changes. It answers with the branch's status report (new keys, source proposals, removed keys, conflicts, outdated per locale). Without values, the branch, commit and pull request come from the CI environment (`GITHUB_HEAD_REF`/`GITHUB_REF_NAME`, `GITHUB_SHA`, `PR_NUMBER`); `--dry-run` doesn't apply to a branch push. |
| `pull` | Writes translations to the catalogs, sorted and deterministic. `--states approved,needs_review\|all`, `--locales`. `--release <id\|v<N>\|latest> [--environment env] [--out dir]` writes a release bundle instead (see *Release*). |
| `extract` | Finds message usages and prints them as a `glossa.usages/v1` document (RFC 0004 §2.2; contract and fixtures: `runtimes/testdata/usages/`). Go is parsed with `go/parser`: `.T(…)` calls (`Client.T(ctx, "…")`, `l.T("…")`, `For(…).T("…")`) and the generated accessors, with the enclosing `pkg.Func` / `pkg.(*Type).Method` as component; files starting with `// Code generated … DO NOT EDIT.` are skipped. Go templates (`extract.templates`) are parsed with `text/template/parse`: `{{t}}`, `{{td}}`, `{{th}}`. Web files are scanned lexically: `t("…")`/`$t("…")`, `<glossa-text\|rich\|plural\|select key\|message>`, `<GlossaText id>`, `<T id>`, typed accessors; Vue and Astro files are their own component, Astro pages carry their route. Only literal keys count. Reports keys missing from the catalog and catalog messages nothing uses; `--strict` exits 1 on unknown keys. `--upload` sends the document to the project's context builds as an `extract` build (what `context push` does). The document's application is `--application`, `GLOSSA_APPLICATION` or `extract.application`; its commit and branch are `--commit`/`--branch`, `GLOSSA_COMMIT`/`GLOSSA_BRANCH`, GitHub Actions (a pull request's head, not its merge commit) or GitLab CI, else git. |
| `context push <file>` | Uploads a `glossa.usages/v1` document — `@glossa/unplugin`'s `.glossa/usages.json`, or a saved `extract --json` — to the project's context builds (`POST …/context-builds`, RFC 0004 §2). `--source plugin\|extract\|runtime\|capture` names the collector; by default `extract` when the document's tool is `glossa`, else `plugin`. Prints the build and how many usages name keys the catalog doesn't know; the same document again is "Already uploaded" (the server answers with the first build). Whether a build is of the default branch is the project's `default_branch` setting, not the uploader's say. A document the server refuses (`invalid_usages`, `too_many_usages`, `unknown_application`, `invalid_source`, `payload_too_large`) exits 2. |
| `capture` | Screenshots the pages of the capture plan (`capture:` in glossa.yaml) in headless Chrome, with `scout`, at every viewport and locale, and records where each message renders (RFC 0004 §3.1–§3.2): one `glossa.captures/v1` document (schema: `runtimes/testdata/schemas/captures.v1.schema.json`) with a full-page PNG per (route, viewport, locale), in a fixed order (route, URL, locale, the plan's viewport order). Before the page's scripts run it injects `@glossa/capture`'s agent, which hooks every `@glossa/runtime` on the page; after load (and the route's playbook) it waits until the DOM is quiet. It refuses a page whose runtime reports a `production` manifest (`production_page`), has no active release (`environment_unknown`), has no runtime (`no_runtime`) or doesn't render the requested locale (`locale_mismatch`), and blacks out `data-glossa-redact` elements before the screenshot (their regions are `visible: false`). Without `--upload` the manifest (`captures.json`) and the images (`<sha256>.png`) go to `capture.output` or `--out`; with `--upload` they're posted to the Captures API. The coverage report lists the messages with a current usage of the application (the branch's view) but no visible region; it reads the Context API, so `--no-coverage` is needed offline. Application, commit and branch are found like `extract`'s (`capture.application` first). `--cdp` (or `GLOSSA_CAPTURE_CDP`) attaches to a browser you started yourself instead of launching one — see "Attaching to a browser" below. A page that fails to load, a refusal, no Chrome (`no_browser`) or an endpoint it won't attach to (`invalid_cdp_endpoint`, `cdp_unreachable`) exits 2. |
| `generate` | Typed accessors from the catalog's argument metadata. `--check` writes nothing and exits 1 when the files are stale; `--from-server` uses the server's messages. |
| `check` | Structural QA: invalid messages, translation/source compatibility (`messageformat.CheckCompat`), missing and outdated translations. `--offline` checks local catalogs. `--require-complete=de,en\|none`, `--fail-on=error\|warning`. |
| `status` | Coverage per locale: translated, approved, needs review, draft, outdated, missing. One request: the server's `translation-stats`. `--offline` counts the local catalogs. |
| `diff` | Local catalogs vs the server by canonical model (so MF1 spelling changes aren't changes). `--exit-code`. |
| `locales`, `messages`, `namespaces` | Lists. `messages --prefix --namespace --missing-in --outdated-in --state active\|obsolete\|all`; `namespaces`: each namespace with its active and obsolete message counts (`GET …/namespaces`) |
| `import --format xliff\|json\|po\|tmx\|tbx <file>` | Imports an interchange file through the server's import jobs: a dry run unless `--apply` (merge) or `--overwrite`; conflicts and invalid items as `file:line:column` (see *Import and export*). |
| `export --format xliff\|json\|tmx\|tbx` | Exports through the server's export jobs and downloads the file, checked against its SHA-256. `-o`, `--unzip`, `--job` (see *Import and export*). |
| `jobs` | Import and export jobs: `list [--direction --state --all-projects --limit]` (the project's and the workspace's TMX and TBX jobs), `show <id> [--wait] [--all-results]`, `cancel <id>`. |
| `import --from v0` | Imports a Glossa v0.3 project (below). |
| `release` | `publish [--dry-run]`, `list`, `show`, `diff`, `promote`, `rollback`, `environments`, `keys [list\|create\|scope\|revoke]` (see *Release*). |
| `branch` | `status [<name>]`: what the branch proposes — new keys, source proposals, removed keys, key conflicts with other open branches, and the translations per locale merging it will make outdated (exit 1 on a conflict). `close [<name>]`: closes an unmerged branch, which destroys its preview environment; its proposed messages become obsolete 14 days later unless it is reopened. Without `<name>`, the branch comes from `GITHUB_HEAD_REF`/`GITHUB_REF_NAME`. |
| `preview register --url <url>` | Records where CI deployed the branch's preview (`--branch`, else the CI environment's). Studio and the pull request comment link to it. |
| `github connections` | Git connections — which repository feeds which project and application (RFC 0004 §6.1): `list [--project P \| --all-projects] [--installation I]`, `add --repository <id\|owner/name> --application A [--project P] [--path apps/web] [--branch main] [--installation I]`, `remove <connection-id>` (see *Git connections*). |
| `tm` | Translation memory: `search <text> --to L`, `concordance <text>`, `units [--locale-pair de:en] [--retire <id>]` (see *Knowledge and AI*); `export` / `import <file>`: TMX (`export`/`import --format tmx`). |
| `terms` | Termbase: `list`, `show`, `add`, `edit`, `deprecate`, `forbid`, and `check`, terminology QA over the project's translations; `export` / `import <file>`: TBX (`export`/`import --format tbx`). |
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
| 1 | A check failed: `check`, `terms check`, `diff --exit-code`, `generate --check`, `extract --strict`, `release publish --dry-run` (not releasable), `translate --dry-run` (a refusal: consent off, no budget, no provider), `import --format` (conflicts or invalid items, dry run or not), `jobs show --wait` (the same for an import) |
| 2 | Usage or configuration: bad flags, missing/invalid glossa.yaml, catalog or style file, unavailable command, input the server rejects as invalid (`invalid_environment`, `invalid_note`, `invalid_key_name`, `idempotency_key_reused`, and every 400 of the Knowledge and Intelligence APIs, e.g. `invalid_locale`, `duplicate_term`), `locale_not_found`, an ambiguous term or key (`term_ambiguous`, `suggestion_ambiguous`), a Git connection the flags can't name (`unknown_repository`, `repository_ambiguous`, `unknown_installation`, `unknown_application`, `project_not_found`, `invalid_connection`) |
| 3 | Network or auth: server unreachable, token missing or refused, forbidden, not found (`term_not_found`, `suggestion_not_found`), server error, or the server refusing the operation (`release_ineligible`, `no_rollback_target`, `not_in_history`, `not_releasable`, `key_revoked`, `storage_unavailable`, `suggestion_decided`, `suggestion_outdated`, `translation_conflict`, `translation_rejected`, `precondition_failed`, `job_not_cancellable`, `upload_not_expected`, `export_not_ready`, `file_expired`, `github_not_configured`, `github_unavailable`, `repository_not_visible`, `application_not_found`, `connection_exists`, `installation_revoked`), `translate --wait`, `import`, `export` or `jobs show --wait` giving up (`wait_timeout`), a transfer that doesn't check out (`upload_corrupted`, `download_corrupted`, `download_interrupted`). Import/export input the server rejects (`invalid_format`, `invalid_options`, `empty_file`, `file_too_large`, …) is 2 |
| 4 | Partial failure: `push` or `import --from v0` went through but some items failed; `translate --wait`: some jobs failed; `import --format`, `export`, `jobs show --wait`: the job failed or was cancelled |

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
| `glossa.cli.push/v1` | `{dry_run, source, branch?: Branch, summary: {created, revised, updated, unchanged, failed}, messages: [{key, status, revision?, state?, error?: {code, detail}}], translations?: [{key, locale, status, state?, error?}]}`; with `--branch` a message's status is `new_key`, `source_proposal`, `unchanged`, `key_conflict` or `failed` |
| `glossa.cli.branch/v1` | `{action (status \| close), branch: Branch}` where Branch is `{id, name, state, pr_number?, head_commit?, preview_url?, new_keys?, source_proposals?, removed?, conflicts?: [{key, branches}], outdated?: {locale: n}}` |
| `glossa.cli.preview/v1` | `{url, branch: Branch}` |
| `glossa.cli.github.connections.list/v1` | `{project_id (null: the whole workspace), connections: [Connection]}` |
| `glossa.cli.github.connections.add/v1` | `{connection: Connection}` |
| `glossa.cli.github.connections.remove/v1` | `{connection_id}` |
| `glossa.cli.pull/v1` | `{states, locales: [{locale, path, messages, skipped: {state: n}, outdated, changed}], release?: {dir, release_id, version, environment, locales, artifacts, bytes, removed}}` |
| `glossa.usages/v1` | `extract --json` (with or without `--upload`): `{application, commit, branch, tool: {name, version}, usages: [{key, file, line, column, component?, route?, kind}]}`, sorted by key, file, line, column; kind is `t`, `component`, `element`, `accessor` or `template` (the schema: `runtimes/testdata/schemas/usages.v1.schema.json`) |
| `glossa.cli.context.push/v1` | `{file, source, replayed, build: {id, application_id, commit, branch, on_default_branch, source, tool: {name, version}, digest, usages, unknown_keys, created_by, created_at}}` (the API's `ContextBuild`) |
| `glossa.cli.capture/v1` | `{application, commit, branch, captures: [{route, url, locale, viewport: {width, height, deviceScaleFactor?}, image: {sha256, width, height}, renders, regions, visible, redacted, truncated}], output?: {dir, manifest, images}, upload?: {build, captures, images_stored, images_deduplicated, unknown_keys, replayed}, coverage: {messages, captured, not_captured: [{key, usages, file, line, routes?}]} \| null}` (`capture`; the regions themselves are in the manifest) |
| `glossa.cli.generate/v1` | `{source, messages, check, files: [{path, kind, changed}], warnings: [{key, reason}]}` |
| `glossa.cli.check/v1` | `{policy: {require_complete (null = all), fail_on}, origin, messages, invalid_messages, locales: [{code, is_source, required, messages, translated, missing, outdated, errors, warnings, complete}], findings: [{check, code, severity, locale?, key?, subject?, detail?, message, where?}], errors, warnings, passed}` |
| `glossa.cli.status/v1` | `{origin, messages, locales: [{code, direction, is_source, translated, approved, needs_review, draft, rejected, outdated, missing, coverage}]}` |
| `glossa.cli.diff/v1` | `{source: {locale, added, changed: [{key, local, server}], removed, unchanged}, translations: [same], identical}` |
| `glossa.cli.locales/v1` | `{locales: [{code, direction, is_source}], fallback}` |
| `glossa.cli.messages/v1` | `{messages: [{key, namespace, state, source_revision, text, syntax, arguments: [{name, type}], description?}]}` |
| `glossa.cli.namespaces/v1` | `{namespaces: [{name, active_messages, obsolete_messages}]}` (by name) |
| `glossa.cli.import/v1` | `{from, source: {url, project}, dry_run, locales_added, summary: {message: {status: n}, translation: {status: n}}, items: [{kind, key, locale, status, v0_status?, state?, downgraded?, reason?, error?}]}` (`--from v0`) |
| `glossa.cli.import.job/v1` | `{format, mode (dry_run \| merge \| overwrite), dry_run, scope (project \| tenant), file: {path, size, sha256}, job: Job, waited, results: [Result], results_filter (problems \| all)}` (`import --format`, `tm import`, `terms import`) |
| `glossa.cli.export/v1` | `{format, scope, job: Job, waited, file: {path?, name, size, sha256, content_type, verified} \| null, extracted: [{path, size}]}` (`export`, `tm export`, `terms export`) |
| `glossa.cli.jobs.list/v1` | `{jobs: [Job]}` (newest first; the project's and the workspace's, or with `--all-projects` the tenant's) |
| `glossa.cli.jobs.show/v1` | `{job: Job, results: [Result], results_filter}` |
| `glossa.cli.jobs.cancel/v1` | `{job: Job}` |
| `glossa.cli.release.publish/v1` | `{replayed, idempotency_key, release: Release}` |
| `glossa.cli.release.preview/v1` | `{environment, policy: {states, include_outdated}, base: Ref \| null, releasable, problems: [{code, detail, key?, locale?}], release: {source_locale, locales: [code], manifest_digest, counts} \| null, changes: [{locale, added, changed, removed}], identical}` (`release publish --dry-run`) |
| `glossa.cli.release.list/v1` | `{releases: [Release & {serving: [environment]}]}` (newest first) |
| `glossa.cli.release.show/v1` | `{release: Release, serving: [environment]}` |
| `glossa.cli.release.diff/v1` | `{release: Ref, base: Ref \| null, locales: [{locale, added, changed, removed}], identical}` |
| `glossa.cli.release.promote/v1`, `glossa.cli.release.rollback/v1` | `{environment: Environment, previous: Ref \| null}` |
| `glossa.cli.release.environments/v1` | `{environments: [Environment]}` |
| `glossa.cli.release.keys/v1` | `{keys: [DeliveryKey]}` |
| `glossa.cli.release.key/v1` | `{action: created \| scoped \| revoked, key: DeliveryKey}` |

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

The import and export shapes share:

- `Job`: `{id, direction (import \| export), kind (catalog \| tm \| termbase), format, mode? (imports), state (awaiting_upload \| queued \| running \| succeeded \| failed \| cancelled), project_id (null: tenant-wide), file_name, file?: {size, sha256, content_type}, options (the API's ImportOptions or ExportOptions), total_items?, processed_items?, summary?: {created, updated, unchanged, conflict, invalid, by_kind: {kind: counts}} (imports), written? (exports), reused_job_id?, failure_code?, failure_message?, cancel_requested, created_by, created_at, started_at?, finished_at?, expires_at, files_deleted_at?}`
- `Result`: `{seq, kind (message \| translation \| tm_unit \| concept), key, locale?, status (created \| updated \| unchanged \| conflict \| invalid), code?, detail?, line?, column?, ref? (the item in the format's own terms: an XLIFF fragment identifier `#/f=…/u=…`, a JSON pointer, `msgctxt "…" msgid "…"`, `tu[n]`, `conceptEntry[n]`), location? (file:line:column)}`

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
- `DeliveryKey`: `{id, name, key, scope: {environments, branches}, created_at, revoked_at?}`

The Git connection shapes share:

- `Connection`: `{id, installation_id, repository_id (GitHub's numeric id), repository_name (owner/name, a label GitHub may change), project_id, project? (the slug, left out when it couldn't be read), application_id, application?, default_branch, path ("": the whole repository), created_by?, created_at, updated_at?, version}`

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

// glossa-react.ts
declare module "@glossa/react" { interface GlossaRegister { messages: Messages } }
export function useTypedMessages() { … }              // a hook: m.cart.items({ count }); <T id> is typed too
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

## Import and export

Interchange files go through the server's import and export jobs (RFC
0003 §6; platform/README.md *Integration*): XLIFF 2.1, flat or nested
JSON, gettext PO (import only), TMX 1.4b and TBX-Basic.

```sh
glossa import --format xliff translations/de.xlf            # dry run: what would happen, nothing written
glossa import --format xliff translations/de.xlf --apply    # merge
glossa import --format json locales/fr.json --locale fr --apply
glossa import --format po po/de.po --state needs_review --apply
glossa import --format xliff vendor/fr-CA.xlf --locale fr --apply  # the file's trgLang says fr-CA; the project has fr
glossa export --format xliff --locale de,fr -o out --unzip  # one XLIFF per locale
glossa export --format json --locale de --layout nested -o locales/de.json
glossa tm export -o memory.tmx          && glossa tm import vendor.tmx --apply
glossa terms export -o termbase.tbx     && glossa terms import glossary.tbx --apply
glossa tm import agency.tmx --scope tenant --apply                 # the workspace's memory (tm-import-jobs)
glossa namespaces                                                  # what --namespace can take
glossa jobs list && glossa jobs show <id> && glossa jobs cancel <id>
```

```text
Imported translations/de.xlf (xliff, 12.4 KiB) into brotwerk
✓ messages: 42 unchanged
✗ translations: 3 created · 38 unchanged · 1 conflict
translations/de.xlf:118:9: conflict translation de checkout.pay  approved_translation_conflict: …
```

- **Dry run by default.** `import --format` writes nothing unless
  `--apply` (mode `merge`) or `--overwrite` (the file's state wins;
  needs `integration.manage`) says so; `--dry-run` says it explicitly.
  The server's own default is `merge`, but a CLI step in CI that
  forgot a flag should report, not write: the dry run runs every check
  a merge runs (structure, the approval rule, concept and unit rules)
  and stores only its results, so a CI job can gate on it (exit 1) and
  a second step applies. `import --from v0` keeps its own contract
  (`--dry-run` to preview); `--from` and `--format` exclude each other,
  and each refuses the other's flags.
- **Flow.** Create the job (with a fresh `Idempotency-Key`, so a retried
  request creates one job) → `PUT` the file to its `upload_url` (streamed,
  progress on stderr on a terminal, `uploaded <name>` otherwise; the
  server's recorded SHA-256 must match what was sent, else
  `upload_corrupted` and the job is cancelled) → poll (`--poll-interval
  1s`, `--timeout 30m`, progress on stderr) → read the results, a page of
  100 at a time. `--no-wait` returns once the file is uploaded (the export
  once it is queued); `glossa jobs show <id> --wait` and `glossa export
  --job <id> -o …` pick up from there. Progress never goes to stdout, and
  not at all with `--json` or `--quiet`.
- **Results.** Conflicts and invalid items are listed in file order as
  `file:line:column: status kind [locale] key  code: detail` (the path as
  given, so editors and CI annotations jump to it) — every item, since
  the server records where each entry of the file is; `--json` adds
  `ref`, the item in the format's own terms (an XLIFF fragment
  identifier, a JSON pointer, a PO `msgctxt` and `msgid`, `tu[n]`).
  `--all-results` lists every item (`--json`: `results`). The summary counts
  created/updated/unchanged/conflict/invalid per kind (messages,
  translations, TM units, concepts). An applied import of a file and
  options already applied successfully reuses that job's result
  (`reused_job_id`) instead of applying it twice; dry runs always run.
- **Options per format.** The CLI refuses options a format doesn't take
  (exit 2) before creating a job:

  | Format | Import | Export |
  |---|---|---|
  | `xliff` | `--locale L` (the translations' locale, default its `trgLang`: names it for a file without one, or imports a file as another of the project's locales), `--syntax mf1` (read other tools' plain units as ICU) | `--locale L…` (one document per target locale; none: the source only), `--namespace N…`, `--state S…` |
  | `json` | `--locale L` (default the source locale: a source catalog, needs `integration.manage`), `--namespace N`, `--syntax mf1\|mf2` (default glossa.yaml's `syntax`), `--state S` (default `needs_review`) | `--locale L…` (default the source), `--namespace N…`, `--state S…`, `--layout flat\|nested`, `--syntax` |
  | `po` | `--locale L` (default its `Language` header), `--namespace N`, `--state S` (entries without `fuzzy`; default `approved`), `--plural-variable V` | — (import only) |
  | `tmx` | `--scope project\|tenant` | `--locale L…` (targets), `--source-locale L`, `--scope` |
  | `tbx` | `--scope project\|tenant` | `--scope` |

  `--locale`, `--namespace` and `--state` repeat or take commas on
  exports (`glossa namespaces` lists the project's); `--state` defaults
  to `approved`. An import's `--locale` must be one of the project's
  locales (`locale_not_found`, exit 2); a file whose translations are in
  a locale the project lacks fails `target_locale_mismatch` (exit 4)
  naming the project's locales — import it again with `--locale`.
  `--scope tenant` goes through the workspace's own routes
  (`tm-import-jobs`, `tm-export-jobs`, `termbase-import-jobs`,
  `termbase-export-jobs`): TMX and TBX imported tenant-wide, every unit
  or concept of the tenant exported; catalogs always belong to the
  project.
- **Downloads are verified.** `export` streams the file into a temporary
  file next to its destination while hashing it, compares the SHA-256
  with the `ETag` and the job's recorded digest, and only then renames it
  into place: a corrupted or interrupted download writes nothing (exit 3,
  `download_corrupted`). `-o` names the file (default: the server's file
  name in the working directory; an existing directory takes the file by
  that name). Several locales come as one zip (`<locale>.<ext>`
  entries); `--unzip -o <dir>` extracts it, refusing entries that would
  land outside the directory. A job's file is kept for a while (7 days by
  default); after that the download is `file_expired`.
- **Exit codes.** `import --format`: 0 ok, 1 conflicts or invalid items,
  2 usage or input the server rejects, 3 refused (permissions, a
  finished or expired job, the wait timing out, a corrupted transfer), 4
  the job failed (a malformed file: its problem is the last result, with
  line and column) or was cancelled. `export`: the same without 1.
- **Permissions.** A `read` token may export and see jobs; imports need
  `write` (translations of catalogs, within the token's locales); TMX,
  TBX, source catalogs and `--overwrite` need `integration.manage`, which
  `write` tokens hold. The job applies the access its requester had when
  they asked.

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
  project and read-only. A new key reads `production` only;
  `--environments a,b` names the environments it reads and `--preview`
  adds every branch preview (RFC 0004 §4.3), which is what a key in a
  preview deployment needs. `keys scope <id|name>` changes that later
  without changing the key, so bundles that ship it keep working, and
  `keys revoke` takes the ID or the name of an active key.
- `pull --release` writes the bundle runtimes load offline
  (runtimes/SPEC.md §3.4): `manifest.json` exactly as the edge serves it
  for `--environment`, and `a/<sha256>.json` for every artifact, each
  checked against its hash, the manifest last; artifacts of an earlier
  bundle it no longer names are removed. Runtimes reject a manifest for
  another environment than theirs, so the bundle is written for one:
  `latest` is what `--environment` serves now; a release ID or `v<N>`
  defaults to the environment it was published to.

## Git connections

A Git connection ties one of a GitHub App installation's repositories —
by GitHub's numeric id, so a rename or a transfer doesn't break it — to
a project, one of its applications, a default branch and an optional
monorepo path (RFC 0004 §6.1). It is what lets Glossa check that
repository's pull requests and comment on them, and what a GitHub
Actions run's ID token is matched against when it authenticates without
a secret (*Authentication*). One repository can feed several projects,
one per path — which is when a CI login has to name one.

Connecting a repository normally happens in **Studio**, which walks
through installing the App on the account and picking the project;
`glossa github connections` is the scripting path, for a workspace that
provisions its projects from CI. The CLI never installs the App: it can
only connect repositories an installation already sees.

- Connections belong to the workspace, not to one project, so `list`
  shows the configured project's (`--project P` another's, by slug or
  ID) and `--all-projects` widens it to every project. It pages through
  the whole list, not just the first page.
- `add --repository` takes GitHub's numeric repository id or
  `owner/name`. A name is resolved through the installations, which is
  what makes the command usable by hand; when several installations see
  a repository of that name it is refused as `repository_ambiguous`,
  naming the accounts, and `--installation <id|account>` picks one.
  `--path apps/web` limits the connection to a monorepo subdirectory
  (left out: the whole repository), `--branch` overrides the
  repository's default branch on GitHub, and `--application` takes an
  application's slug or ID — one the project doesn't have is refused
  here rather than failing quietly on the first pull request.
- `remove <connection-id>` only unlinks the repository: Glossa stops
  checking its pull requests, and the App **stays installed on GitHub**.
  Only GitHub can uninstall it, on the account's or organization's
  Applications settings page.
- A deployment with no GitHub App configured answers
  `github_not_configured` (exit 3) to every one of these commands.

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

## Attaching to a browser

`glossa capture` starts headless Chrome itself. Where it can't — a CI
sandbox whose kernel denies Chrome's own sandbox, a container or a
remote browser — start the browser yourself and point capture at its
DevTools endpoint:

```bash
google-chrome --headless=new --no-sandbox --disable-dev-shm-usage \
  --remote-debugging-port=9222 --user-data-dir="$(mktemp -d)" about:blank &

glossa capture --cdp http://127.0.0.1:9222     # or: export GLOSSA_CAPTURE_CDP=…
```

The endpoint is `ws://…`/`wss://…` (a browser WebSocket, as
`/json/version` reports it) or `http://…`/`https://…`, which capture
asks for that WebSocket. It keeps the endpoint's own host and port and
takes only the path, so a browser behind a tunnel or a published
container port works.

**The host must be a loopback address** (`127.0.0.1`, `::1`,
`localhost`). A DevTools endpoint is full control of that browser — its
pages, their cookies and every address it can reach — and this value
arrives from a flag or a CI variable, so capture refuses anything else
(`invalid_cdp_endpoint`) before it connects. `--cdp-allow-remote` (or
`GLOSSA_CAPTURE_CDP_ALLOW_REMOTE=1`) lifts that for a browser on another
host; then the endpoint is yours to trust.

Attached, capture owns only the tabs it opens: each is closed after its
capture, and the browser keeps running afterwards.

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
- `extract` finds literal keys, not computed ones, and doesn't read
  `.gitignore` (it skips hidden directories, `node_modules`, `dist`,
  `vendor`, `build` and `coverage`). Its web scan is lexical, the fallback
  when `@glossa/unplugin` doesn't run: it doesn't skip comments or
  strings, and JSX files get no component. A Go file or template that
  doesn't parse is reported and skipped. Go accessor calls on an imported
  package (`strings.Title`) don't count; an unaliased import whose package
  name differs from its path is guessed from the path.
- `extract --upload` and `context push` post the document to
  `POST /v1/tenants/{tenant}/projects/{project}/context-builds?source=…`;
  the upload is idempotent by application, commit, source and the
  digest of the document's canonical form, and the server rate-limits
  uploads per tenant (10 a minute, bursts of 60; 429 is retried).
  `context push` reads only the document's header before sending it; the
  server validates the rest by the schema.
- `capture` needs Chrome or Chromium on the machine (scout looks for
  `google-chrome`, `chromium` and the usual app paths; GitHub's Ubuntu
  runners have it), or a browser to attach to with `--cdp`. scout's
  launcher passes a fixed flag list, so a sandbox that needs
  `--no-sandbox` or `--disable-dev-shm-usage` is the attach path's job
  (platform/README.md, "scout follow-ups"). It lays pages out with
  overlay scrollbars, so an
  image is as wide as its viewport; a page taller than 16 384 device
  pixels or 40 megapixels is cut there (`truncated`), and its regions
  below the cut are `visible: false`. A PNG over 10 MB fails the capture.
  Pages are captured one at a time, each on a fresh tab. A playbook's own
  starting URL is ignored: it starts on the route's page. Upload:
  `POST …/captures`, multipart, the `manifest` part first, then one
  `image/png` part per distinct image named by its SHA-256.
- Windows stores tokens in the file store.
- `translate --dry-run`'s `tm_exact` counts exact translation-memory
  matches; the job still validates one before reusing it (structure,
  plural categories, terminology), so a match that fails there goes to a
  provider after all. Its cost is an estimate from the price table, not
  a quote.
- `import --format` reads a file from disk (no `-`/stdin): the upload
  sends its size up front.
