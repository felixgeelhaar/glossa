# M4 exit test — report

Written by `TestM4Exit` (`make system-m4`, or `go test -tags=system ./internal/systemtest/m4/...` in
`platform/`) against a real glossa-server on Postgres and MinIO, a fake GitHub on loopback, a headless
Chrome the test starts, the Dart SDK, and Studio's own Playwright suite pointed at the same server.
Every number below comes from the public API, from GitHub's own view of the pull request, from the
browser, or from the runtime's own test suites. RFC 0005 §12.

## The verdict

**4 of the 8 exit criteria hold.**

| § | Criterion | Verdict |
|---|---|---|
| 12.1 | A fixture repository with real CI | met |
| 12.2 | Findings across layers | **not met** |
| 12.3 | The PR check agrees | **not met** |
| 12.4 | Waivers and policy rollout | **not met** |
| 12.5 | Release gate | **not met** |
| 12.6 | MCP | met |
| 12.7 | Flutter | met |
| 12.8 | Dashboard | met |

What is missing, in one line each:

- **§12.2**: the region the finding names cannot be read back and cropped, because **no visual finding ever reaches the server**: `glossa capture --upload` writes a `glossa.captures/v1` manifest whose `Capture` struct (cli/capture/document.go) has `route`, `url`, `viewport`, `locale`, `image`, `renders` and `regions` — and no `findings`. The probe pass measures them (the run above found the clip), `glossa capture --check` grades them locally, and the upload drops them. The server is ready for them: `CapturesManifestFinding` is in the API and `context/app.recordFindings` hands them to Quality, but `up.FindingCount()` is always 0. The server stored 0 visual findings, and 0 probe findings were reported as uploaded
  - structure: no finding. `structure` reports text that did not survive parsing, and no write path can store such text: localization/app.Service.prepare parses every translation and QAResult.Gate refuses one with error-severity findings, and `glossa push` refuses an invalid source message. The layer is therefore unreachable against a server project and reachable only from `glossa check --offline` over local catalogs (which this test also runs, below)
  - completeness: no finding with `unknown-key`
  - style: no finding, because the layer does not exist: there is no `quality/layers/style.go`, `layers.Default()` returns Structure, Parity and Completeness only, and nothing anywhere emits `formality-mismatch`. RFC 0005 §13's wave-2 slice ("`style` layer over the effective style guide") has not landed
  - length: no finding under this layer. The only length rule that exists is `max-length-exceeded`, computed by localization/domain.CheckStructure when a translation is written and surfaced by the **parity** layer from the stored warning — so the one case that works is reported under the wrong layer, and `expansion-excessive` and `layout-overflow-predicted` are not computed at all. RFC 0005 §13's wave-1 `length` slice has not landed
  - locale: no finding, because the layer does not exist: there is no `quality/layers/locale.go` and nothing emits `number-convention` or `bidi-stray-control`. RFC 0005 §13's wave-1 `locale` slice has not landed
  - source: no finding, because the layer does not exist: there is no `quality/layers/source.go` and nothing emits `manual-plural` or `ambiguous-short`. RFC 0005 §13's wave-1 `source` slice has not landed
  - no `unknown-key` finding names `src/pages/CheckoutPage.vue:31`. `glossa check`'s completeness layer emits `unknown-key` only for a stored translation whose key has no source message (quality/layers/completeness.go), and its locus carries no file or line, because the CLI does not enrich a locus from Context. The finding §12.2 describes — a usage of a key the catalog does not have, with its `file:line` — is emitted only by the pull-request check, from the usages document (integration/app/check_report.go's `findings`)
  - every terminology finding carries an empty `locus.namespace`, so policy v3's rule `{layer: terminology, namespace: legal, severity: error}` can never select one: `checkpolicy.Selector` matches on `locus.namespace`, and the terminology layer does not set it (cli/terminology and the server's termbase check answer by key and locale). The `legal` namespace's terminology is an error here only because `term_forbidden` is already one by default
- **§12.3**: the terminal and the pull request do not reach the same verdict, and the reason is structural rather than a fixture accident. `glossa check` **recomputes every layer over the whole project** — `snapshot.FromServer` reads the active messages and every translation, and `quality/app.RunIn` runs Structure, Parity and Completeness over all of them. The pull-request check **reads what is already stored, for the branch's own keys only**: `integration/adapters/sources.Checks.qaFindings` returns nothing at all when the branch proposes no new key and no source change (`len(branchKeys) == 0`), and otherwise returns the warnings Localization kept when each translation was *written* plus a live terminology check. A parity break a later source revision created is stored nowhere, so the pull request cannot see it; and missing and outdated translations are rolled up to **one finding per locale** on the pull request against **one per message and locale** in the terminal. Two surfaces, two scopes, two arithmetics. The rows above are where that shows.
- **§12.4**: the waived finding is still `warning` in `glossa check`, because **the check never reads the project's waivers**: nothing in `cli/cmd_check.go` fetches them, and `quality/app.RunIn` — the function the CLI, the capture check and Studio all call — takes findings and a policy and no waivers at all. `domain.Waivers` is applied in exactly one place, `quality/app.RecordRun` (runs.go:91), which is the server recording a check run. A waiver therefore changes no local check's counts and no local conclusion, which is what §12.4 asks it to change
  - waived went from 0 to 0, want one more
  - the impact preview says 0 open pull requests would newly fail, want at least the one that is open. It examined 0 stored findings over 0 runs and raised 0 of them; newly-failing refs: []. The preview reads *stored* findings, and the only findings this project has stored are the capture ingest's — so a policy change is previewed against whatever happens to have been recorded, not against what a check would compute
  - the new pull request concluded `success`; with `visual` at `enforce` and a clipped button on its commit it has to fail. It cannot, for the same reason §12.2's crop cannot: the capture manifest has no `findings` field, so no visual finding is ever stored, so the pull-request check has none to grade and `visual: enforce` gates nothing
- **§12.5**: a forced publish with no reason was refused as `policy_not_met`, not `force_reason_required`, because **the publish endpoint does not read `force` at all**: `release/adapters/httpapi.API.PublishRelease` builds `app.PublishInput{Environment, Note}` and drops `req.Body.Force` and `req.Body.ForceReason`, although `app.Service.Publish` implements the override and `openapi.yaml`'s `PublishRelease` carries both fields. No publish can be forced through the API today
  - the forced publish failed: HTTP 409: {"type":"urn:glossa:problem:policy_not_met","title":"Conflict","status":409,"code":"policy_not_met","detail":"release: the environment's check policy is not met: production: fr must be complete and is 3 of 151 messages short"}


## §12.1 — a fixture repository with real CI

`testdata/repo` is the **M3 fixture application** — Brotwerk's shop, 150 messages over eight routes,
source `de`, targets `en`, `es`, `fr`, `ja`, with its committed Vite build — plus what M4 adds: a
`.github/workflows/quality.yml` job that runs `glossa push`, `glossa context push`, `glossa check --json`
and `glossa capture --check --upload`; a capture plan for `/kasse` in German and Japanese; and one
stylesheet that gives the checkout page a fixed-width pay button. The test materializes the repository,
runs the workflow's commands, and never writes to the committed M3 fixture.

The project's check policy is **v3**: `require_complete: [de, en]`, `terminology` an error in the
`legal` namespace, `visual` in `warn`, and a `production` environment that also requires `fr`.

`glossa push --translations` reported 752 created, 0 failed, 0 revised, 0 unchanged, 0 updated.

The seeded defects, and the layer each is for:

| Seed | Layer §12.2 names it under |
|---|---|
| `checkout.payment.activity`: the German source gains `{$amount}`; every locale but French follows it | `parity` |
| `checkout.roll.help`: the German source drops its `{#link}`; every locale but Japanese follows it | `parity` |
| three French translations that were never written (`cart.flour.title`, `cart.shipping.title`, `cart.tray.title`) | `completeness` |
| two German sources that move under their Japanese (`home.allergen.title`, `products.bun.title`) | `completeness` |
| `checkout.pickup.reminder`, used at `src/pages/CheckoutPage.vue:31` and in no catalog | `completeness` |
| `legal.privacy.notice` (namespace `legal`), whose French uses the forbidden `vie privée` | `terminology` |
| `cart.basket.title`, whose Spanish does not use the preferred `carrito` | `terminology` |
| `checkout.bagel.title` with `max_length: 20` and a French translation over it | `length` (see below) |
| the checkout pay button, 104 px wide and single-line, holding `checkout.crust.label` | `visual` |

## §12.2 — findings across layers

`glossa check --terminology --explain-policy --json` graded the project against policy v3 from the server
and exited **1**: conclusion `failure`, **3 errors, 16 warnings, 0 waived** over 151 active messages.

| Locale | Required | Translated | Missing | Outdated | Errors | Warnings | Complete |
|---|---|---:|---:|---:|---:|---:|---|
| `de` _(source)_ | no | 151 | 0 | 0 | 0 | 0 | yes |
| `en` | yes | 151 | 0 | 0 | 0 | 0 | yes |
| `es` | no | 151 | 0 | 0 | 0 | 8 | yes |
| `fr` | no | 148 | 3 | 1 | 2 | 5 | no |
| `ja` | no | 151 | 0 | 3 | 1 | 3 | yes |

The layers the run computed: `completeness`, `parity`, `structure`, `terminology`.

### The nine cases §12.2 names

| Layer | What §12.2 asks for | What this run produced | Codes |
|---|---|---|---|
| ❌ `structure` | one translation whose MF2 doesn't parse | no finding. `structure` reports text that did not survive parsing, and no write path can store such text: localization/app.Service.prepare parses every translation and QAResult.Gate refuses one with error-severity findings, and `glossa push` refuses an invalid source message. The layer is therefore unreachable against a server project and reachable only from `glossa check --offline` over local catalogs (which this test also runs, below) | — |
| ✅ `parity` | one French translation missing `{$amount}`, one Japanese one adding markup the source doesn't have | 2 findings (2 error, 0 warning) | `markup-extra`, `missing-argument` |
| ❌ `completeness` | three missing `fr`, two outdated `ja`, one unknown key with its file:line | no finding with `unknown-key` | `missing-translation`, `outdated-translation` |
| ✅ `terminology` | one `term_forbidden` in `legal` (error), one `term_missing` elsewhere (warning) | 10 findings (1 error, 9 warning) | `term_forbidden`, `term_missing` |
| ❌ `style` | one German translation using `du` under a `Sie` guide | no finding, because the layer does not exist: there is no `quality/layers/style.go`, `layers.Default()` returns Structure, Parity and Completeness only, and nothing anywhere emits `formality-mismatch`. RFC 0005 §13's wave-2 slice ("`style` layer over the effective style guide") has not landed | — |
| ❌ `length` | one French button over its `max_length`, one over its region's width | no finding under this layer. The only length rule that exists is `max-length-exceeded`, computed by localization/domain.CheckStructure when a translation is written and surfaced by the **parity** layer from the stored warning — so the one case that works is reported under the wrong layer, and `expansion-excessive` and `layout-overflow-predicted` are not computed at all. RFC 0005 §13's wave-1 `length` slice has not landed | — |
| ❌ `locale` | one French translation writing `1,234.50`, one Arabic one with a stray U+202B | no finding, because the layer does not exist: there is no `quality/layers/locale.go` and nothing emits `number-convention` or `bidi-stray-control`. RFC 0005 §13's wave-1 `locale` slice has not landed | — |
| ❌ `source` | one `3 item(s)` and one `ambiguous-short` | no finding, because the layer does not exist: there is no `quality/layers/source.go` and nothing emits `manual-plural` or `ambiguous-short`. RFC 0005 §13's wave-1 `source` slice has not landed | — |
| ✅ `visual` | a real one: Chrome over the fixture, the Japanese checkout button clips | 1 findings (0 error, 1 warning) | `text-clipped` |

**The `structure` layer, and why a server project cannot show one.** `glossa check --offline` over a local
catalog holding text that does not parse produced **1** structure finding(s) (`invalid-translation`), so the layer works.
It cannot appear against the server, and that is not a fixture problem: every write path parses first —
`localization/app.Service.prepare` calls `mfcontent.Parse` and `QAResult.Gate` refuses a translation with
error-severity findings, and `glossa push` refuses an invalid source message — so no stored message or
translation can fail to parse. The layer is reachable only offline today.

**The visual layer.** no region was cropped: the server stored 0 visual findings, so there is no region to read back.

- `glossa context push` uploaded 151 usages, one of them `checkout.pickup.reminder` at `src/pages/CheckoutPage.vue:31` — a key no catalog has.
- The head commit revises 4 German sources; the French `checkout.payment.activity` keeps its text and loses `{$amount}`, and the Japanese `checkout.roll.help` keeps the link the source dropped.
- The termbase holds `Datenschutz` → forbidden French `vie privée` (legal) and `Warenkorb` → preferred Spanish `carrito`.
- `glossa check --json` exited 1 with conclusion `failure`: 3 errors, 16 warnings, 0 waived, graded against policy v3 from the server.
- All 19 findings validate against `glossa.finding/v1` (schema, fingerprint, layer, code, severity, locus).
- The visual layer is real: `glossa capture --check` drove Chrome over `/kasse` in German and Japanese at 1280×800, and the Japanese pay button clipped — 1 `text-clipped` finding(s) on the Japanese capture (0 on the German one), and the server stored 0 probe findings with the upload.
- No single command computes all nine layers: `glossa check --terminology` has the terminology layer and no browser, `glossa capture --check` has the visual layer and takes no `--terminology` flag. The table below counts both runs of the workflow together.

## §12.3 — the PR check agrees (the exit criterion)

Pull request #11 on `acme/shop`, head `6b28d05`, opened by a signed `pull_request.opened` webhook the fake GitHub's
fixtures sign. Nothing creates a check run through `/v1` — `openCheck` is reached only from webhook
processing — so the test does what a product does and assumes no API shortcut.

The check run: `Glossa`, completed/`success`, "1 warning", 1 annotations over 50 PATCHes, 1 sticky comment.

The terminal's side is `glossa capture --check` — the workflow's run that carries every layer the server can have.

| | the terminal | the pull request's check run | |
|---|---|---|---|
| conclusion | failure | success | ❌ |
| errors | 2 | 0 | ❌ |
| warnings | 8 | 1 | ❌ |
| waived | 0 | 0 | ✅ |
| layer `completeness` (e/w/x) | 0/7/0 | 0/1/0 | ❌ |
| layer `parity` (e/w/x) | 2/0/0 | 0/0/0 | ❌ |
| layer `visual` (e/w/x) | 0/1/0 | 0/0/0 | ❌ |

The two surfaces do not agree. That is the criterion M4 is decided by, and it is the row(s) marked ❌
above that decide it.

## §12.4 — waivers and the policy rollout

### The waiver

| What | What happened |
|---|---|
| a waiver with a blank reason | 400 `waiver_reason_required` — the reason is required and non-empty |
| waive `term_missing` (`f_16a7c`) with a reason | waiver `01a0f32`, scope `project`, against source revision 0 |
| re-run `glossa check` | the finding is `warning`; 16→16 warnings, 0→0 waived, conclusion `failure`→`failure` |
| change the German source behind it | the finding is `warning` again: the waiver was made against source revision 0 and the finding is now at 0 |

### The rollout

| What | What happened |
|---|---|
| `POST …/check-policy` with `dry_run: true` | 0 stored findings examined over 0 runs: 0 raised, 0 lowered, 0 silenced; 0 refs newly failing (—); **0 open pull requests** would newly fail |
| save v4 with a 14-day grace | v3 keeps grading the pull requests opened before it, until 2026-10-14T16:38:59.651995Z |
| the open pull request (`feature/checkout-copy`, opened under v3) | still graded against v3, and its summary says so: “Graded against the project's check policy v3 — the version this pull request was opened under.” |

The preview counts the pull requests that would newly fail and names the **refs**, not the pull requests: `CheckPolicyImpact` carries `newly_failing_refs` (strings) and `open_pull_requests` (an integer). §12.4 asks the preview to *name* the one pull request that would newly fail; today a reader gets its branch and a count, and has to look the number up. The ref→PR mapping exists server-side (`Catalog.OpenPullRequests`) and is used only to compute the count.

The preview counted 0 open pull requests, not one.

## §12.5 — the release gate

| What | What happened |
|---|---|
| publish to `production` with `fr` incomplete | 409 `policy_not_met` — release: the environment's check policy is not met: production: fr must be complete and is 3 of 151 messages short |

## §12.6 — MCP

An `mcp-go` client (`github.com/modelcontextprotocol/go-sdk`) over streamable HTTP at `/mcp`, on the same
server, with `GLOSSA_MCP_ENABLED=true` — the endpoint is off by default and this deployment turns it on.

| Credential | Session | Call | Outcome |
|---|---|---|---|
| a CI token (`glossa_ci_…`) | read (asked for) | `connect` | **refused at connect** — HTTP 401:  invalid token: mcp: credential not accepted: a CI token is minted for one workflow run; present a tenant API token (glossa_api_…) instead |
| a live in-context grant (`glossa_ctx_…`) | read (asked for) | `connect` | **refused at connect** — HTTP 401:  invalid token: mcp: credential not accepted: an in-context grant is minted for one project and one browser origin; present a tenant API token (glossa_api_…) instead |
| read-only API token | read | `tools/list` | 11 tools, none of them a write tool |
| read-only API token | read | `catalog_search` | the project's `checkout.` messages |
| read-only API token | read | `message_get` | the message with its usages and capture count |
| read-only API token | read | `check_run` | findings and the policy verdict, computed and stored nowhere |
| read-only API token | read | `translation_propose` | **refused** — calling "tools/call": unknown tool "translation_propose" |
| read-only API token | write (asked for) | `connect` | **refused** — a write session needs the `write` scope |
| write API token | write | `translation_propose` | written, state `needs_review`, origin `agent` — in review, not an approved revision |
| another tenant's API token | read | `catalog_search (the first tenant's project)` | **refused** — mcp: not found |

The CI token above is a well-formed `glossa_ci_…` secret rather than one minted through the GitHub OIDC
exchange: the refusal is by credential kind in `mcp/adapters/identity.Authenticate`, before any lookup, so
that is the path being exercised. The in-context grant **is** a live one, minted through
`POST …/in-context-grants` for the fixture deployment's origin and refused all the same.

## §12.7 — the Flutter runtime

`Dart SDK version: 3.13.4 (stable) (Tue Sep 15 01:01:15 2026 -0700) on "macos_arm64"` · Flutter 3.47.5 • channel stable • https://github.com/flutter/flutter.git.

| Suite | Fixtures | Passed | Failed | Skipped |
|---|---|---:|---:|---:|
| `catalog_test.dart` | `explain()` has no side effects; the SPEC's wire spellings | 9 | 0 | 0 |
| `ed25519_test.dart` | RFC 8032 signature vectors and their rejections | 9 | 0 | 0 |
| `jcs_test.dart` | RFC 8785 canonicalization vectors | 52 | 0 | 0 |
| `loader_test.dart` | the §3 loading cases a fixture cannot ship | 12 | 0 | 0 |
| `loading_test.dart` | runtimes/testdata/loading — including `signatures.json`: an unsigned manifest with keys configured | 7 | 0 | 0 |
| `locale_test.dart` | the BCP 47 suites the Go and JS runtimes share | 37 | 0 | 0 |
| `markup_test.dart` | runtimes/testdata/markup.json | 22 | 0 | 0 |
| `purity_test.dart` | §6.4's AOT and web rules: no `dart:mirrors`, no `dart:ffi`, no `dart:io` in the core | 4 | 0 | 0 |
| `runtime_format_test.dart` | messageformat/testdata/glossa/runtime-format.json | 131 | 0 | 41 |
| `scenarios_test.dart` | runtimes/testdata/scenarios — and `explain()` field for field (SPEC §6) | 24 | 0 | 0 |
| **Total** | | **307** | **0** | **41** |

`explain()` is asserted field for field against SPEC §6 inside `scenarios_test.dart` — locale, chain,
`resolvedFrom`, release id and version, source and every step — and the unsigned manifest is
`runtimes/testdata/loading/signatures.json`'s "release 2 unsigned" step, which runs with public keys
configured and expects a `signature` error and the previous release still serving.

### The §6.4 budgets

**Size** — `flutter build --analyze-size` against a fixture app with and without the package, which is
the method §6.4 names; gated on the delta over a realistic baseline at ≤ 400 kB (§15 question 6).

```
Glossa Flutter runtime — RFC 0005 §6.4 size budget
target: macos
  plain: 3046.6 kB of Dart code
  host: 3752.4 kB of Dart code
  glossa: 4095.4 kB of Dart code

— the fixture is honest ————————————————————————————————————
  bytes of ours in plain, which does not use us: 0
  bytes of ours in host, which does not use us: 0
  bytes of ours in glossa, which does: 133656

— enforced: §6.4's own method, the delta ————————————————————
  adopting Glossa, over a realistic baseline   343.1 kB
    budget 400.0 kB (§6.4, set in wave 4)
    The baseline already has package:intl formatting numbers and
    dates and an HttpClient making a call — what most apps that
    would adopt Glossa already carry.

— reported, because a hidden megabyte is a lie ————————————————
  over a bare Flutter app (no intl, no networking)  1048.8 kB
  the same, excluding package:intl             879.8 kB
  package:intl (its own CLDR data)             169.0 kB
  package:glossa + package:glossa_flutter, attributed   133.7 kB
    The first of these is what an app with neither would pay, and
    it is the number intent §33 is about. The gap between it and
    the gated one is the dart:io HTTP client a transport needs,
    package:intl, the dart:core BigInt arithmetic behind the
    pure-Dart Ed25519 verifier, and package:crypto — carried by
    the baseline above because an app of this kind already has
    them. Per architecture: one slice, whatever the bundle packs.

  where the growth over a bare app went, by library:
      239.6 kB  @shared
      169.0 kB  package:intl
      124.6 kB  package:glossa
      120.7 kB  dart:io
      108.8 kB  dart:_http
       82.2 kB  dart:core
       74.0 kB  @unknown
       41.5 kB  dart:async
       17.9 kB  dart:typed_data
       15.6 kB  package:crypto
       12.2 kB  package:flutter
       10.4 kB  dart:convert
        9.8 kB  dart:_internal
        9.1 kB  package:glossa_flutter/package:glossa_flutter/src
        5.0 kB  @stubs
        2.3 kB  dart:collection
        1.8 kB  dart:isolate
        1.7 kB  package:glossa_size_fixture/package:glossa_size_fixture/glossa.dart
        1.4 kB  dart:_compact_hash

OK — 343.1 kB of 400.0 kB, the delta over a baseline that already has package:intl and an HTTP client. That verdict does not stand alone:
  What it assumes: an app that already makes HTTP calls and already formats numbers and dates. An app with neither pays 1048.8 kB — 879.8 kB of it outside package:intl.
  What the budget is: 150 kB was §6.4's original figure and nothing meets it by this method. Wave 4 replaced it with 400.0 kB deliberately, after measuring. See RFC 0005 §6.4 and §15, question 6, which records the decision and its reasoning.
```

Measured in 130 s on this machine.

**Startup** — measured and recorded, not gated on wall clock: §6.4's numbers are written for a mid-range
Android device and neither a laptop nor a CI runner is one. What the tool does enforce are the two
properties that hold on any machine.

```
Glossa Dart runtime — RFC 0005 §6.4 startup budgets
host: macos 3.13.4
mode: AOT (release)

manifest under test: 202594 bytes (control: 19621 bytes)
the shared fixture verifies against its own key: yes

warm cache: 500 messages, 87360 artifact bytes on disk
activation from the persisted store: 8.771 ms

— §6.4 device budgets, recorded ————————————————————————————
  verify 200 kB manifest + signature        20.10 ms   (within the 30 ms budget)
  first t() after a warm cache               0.12 ms   (within the 5 ms budget)
  longest event-loop stall on activation     4.81 ms   (within the 16.7 ms budget)
  (steady state after warm-up: verification 22.51 ms, canonicalization 8.59 ms of it.)
  These are wall clock on this machine. §6.4 names a mid-range Android device; neither a laptop nor a CI runner is one, so none of the three fails this program. The device numbers belong to the M4 exit report (§12).

— enforced, because they hold on every machine ——————————————
  canonicalization cost per byte, 202594 B over 19621 B  ×0.97 (linear is ×1.00, ceiling ×3.00)
  first t() ÷ one verification  0.5 % (ceiling 25 %)

OK — both enforced properties hold.
```

Measured in 5 s on this machine.

## §12.8 — the dashboard

| Number (RFC 0005 §8) | What the API reported |
|---|---|
| Coverage: translated / outdated / missing | {messages:604,missing:3,outdated:4,translated:601} |
| Outstanding findings by layer and severity, plus waived | not measured — nothing has been checked in this project yet |
| AI acceptance rate and mean edit distance | {acceptance_rate:0,accepted:0,decisions:0,edited:0,mean_edit_distance:0,rejected:0} |
| Review queue depth and age | {depth:0} |
| Context coverage: usages and visible regions | {active_messages:151,with_region:0,with_usage:150} |
| Lead time, p50/p90 | not measured — the environment "production" has published nothing since 2026-08-31T16:41:26Z |
| Check health: pass rate and median time to a conclusion | {failed:0,latency:{p50_seconds:16.818562499,p90_seconds:18.818042900000002,samples:2},median_seconds:16.818562499,neutra… |

`studio/e2e/m4/quality-exit.spec.ts` signed in against **this** server, opened `/t/…/p/…/quality`, found seven stats in the
health header, and asserted each rendered value against the summary above — computing the expected
rendering from the API's JSON with plain `Intl` rather than by calling Studio's own formatters, so the
comparison cannot agree with itself. A number the API did not measure has to read "Not measured" and
carry `data-measured="false"`; a zero there would be a claim the API never made. (5 s)

## No provider, no network

RFC 0005 §14 decision 2: a check never calls an AI provider, and none of §12's nine layers is the
model-backed one. The tenant's only configured provider is a loopback endpoint that refuses every
request and counts it. **It was asked 0 times.**

The GitHub is the fake of RFC 0004 §12, on loopback; Postgres and MinIO are testcontainers; the browser
is a headless Chrome the test starts and attaches to over CDP. Nothing in this run reaches the network.
