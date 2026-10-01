# M5 exit test — report

Written by `TestM5Exit` (`make system-m5`, or `go test -tags=system ./internal/systemtest/m5/...` in
`platform/`) against a real glossa-server on Postgres and MinIO that sends its mail over SMTP to a
capturing test mailer, a real glossa-edge on the same bucket, a v0.3 server built from `apps/api` on its
own Postgres, and the JS, Go and Dart runtimes. Every observation below comes from the public API, the
edge, a runtime or the CLI. RFC 0006 §12.

This test exists from M5's first wave with every criterion written and red (RFC 0006 §11.1): each later
wave turns some of it green, and its merge note says which. A criterion that cannot hold says what is
missing; the steps after the first missing one are listed as *not reached*.

## The verdict

**1 of the 7 exit criteria hold.**

| § | Criterion | Verdict | Fails if |
|---|---|---|---|
| 12.1 | Two workflows, one event | **not met** | B needs a code path A doesn't; an instance strands short of a final state; the author's or a token's grant counts; Workflow's principal approves anything. |
| 12.2 | Vendor visibility on every surface | **not met** | any generated operation leaks an id outside the assignment, or an operation in the spec has no verdict in the coverage table. |
| 12.3 | Release approvals | **not met** | any pointer moves before the second approval as seen at the edge, or a rollback waits. |
| 12.4 | Staged rollout across three runtimes | **not met** | any runtime disagrees with the generator on any id, the share is outside 9–11 %, or an aborted installation stays on the candidate. Runtimes are compared with the generator, never with each other. |
| 12.5 | Audit export | **not met** | a call the harness recorded has no entry (compared with the harness's own log, not the outbox), an entry has the wrong actor, a tampered export verifies, or a canary leaks. |
| 12.6 | v0.3 imports and renders the same | **not met** | any rendering differs between v0.3's formatter and @glossa/runtime (two implementations that share no code), or a carried field is missing. |
| 12.7 | Earlier exits hold | met | any earlier exit criterion fails. A failure here blocks the M5 verdict whatever 12.1–12.6 say. |

What is missing, in one line each:

- **§12.1**: refuse a definition naming a guard the vocabulary does not have with 422 `invalid_workflow` — saving a workflow definition (POST /v1/tenants/01a0f6ad-7fd0-78f8-9bf2-8b200c36304c/workflow-definitions) does not exist on this server (HTTP 404: {"type":"urn:glossa:problem:not_found","title":"Not Found","status":404,"code":"not_found","detail":"no such resource"})
  - refuse a definition with an unreachable state with 422 `invalid_workflow` — saving a workflow definition (POST /v1/tenants/01a0f6ad-7fd0-78f8-9bf2-8b200c36304c/workflow-definitions) does not exist on this server (HTTP 404: {"type":"urn:glossa:problem:not_found","title":"Not Found","status":404,"code":"not_found","detail":"no such resource"})
  - refuse a definition with a delayed transition with 422 `invalid_workflow` — saving a workflow definition (POST /v1/tenants/01a0f6ad-7fd0-78f8-9bf2-8b200c36304c/workflow-definitions) does not exist on this server (HTTP 404: {"type":"urn:glossa:problem:not_found","title":"Not Found","status":404,"code":"not_found","detail":"no such resource"})
  - §2.1's architecture test passes — `go test -run Architecture ./internal/workflow/...` ran no architecture test: ?   	github.com/felixgeelhaar/glossa/platform/internal/workflow/app	[no test files] / ?   	github.com/felixgeelhaar/glossa/platform/internal/workflow/defaults	[no test files] / ok  	github.com/felixgeelhaar/glossa/platform/internal/workflow/domain	0.247s [no tests to run]
  - save `vendor-then-four-eyes` (testdata) through the API — saving a workflow definition (POST /v1/tenants/01a0f6ad-7fd0-78f8-9bf2-8b200c36304c/workflow-definitions) does not exist on this server (HTTP 404: {"type":"urn:glossa:problem:not_found","title":"Not Found","status":404,"code":"not_found","detail":"no such resource"})
- **§12.2**: creating the vendor — POST /v1/tenants/01a0f6ad-7fd0-78f8-9bf2-8b200c36304c/vendors does not exist on this server (HTTP 404: {"type":"urn:glossa:problem:not_found","title":"Not Found","status":404,"code":"not_found","detail":"no such resource"})
  - the vendor's translator was invited, but the member the API returned has visibility "", not "assigned": the field was dropped, so the platform restricts nothing
  - the vendor exists and its translator is a vendor member with visibility `assigned`, scoped to project B — the platform could not create a vendor (POST /v1/tenants/01a0f6ad-7fd0-78f8-9bf2-8b200c36304c/vendors); the member is an ordinary `de` translator
  - assign 20 `de` units of project B to the vendor — creating an assignment (POST /v1/tenants/01a0f6ad-7fd0-78f8-9bf2-8b200c36304c/assignments) does not exist on this server (HTTP 404: {"type":"urn:glossa:problem:not_found","title":"Not Found","status":404,"code":"not_found","detail":"no such resource"})
  - the generated sweep: 57 of 99 GET operations show the vendor member something outside the assignment (first: listExportJobs, getExportJob, downloadExportFile, listProjects, getProject, … 52 more) — reads are not filtered through `Assignments.Covers`
  - the generated sweep: 13 operations have no verdict because the fixture had no id to address them (getAIFill, getAIJob, getAISuggestion, getGitConnection, getImportJob, listImportResults, getBranch, listBranchProposals, … 5 more); an operation with no verdict is a failure
  - writes outside the assignment are refused — the vendor member wrote `b.unit.21` and `a.unit.01`, outside the assignment
  - export, import and TM search are refused — the vendor member was not refused: an export job (201), an import job (201), `GET /tm-concordance` (200)
- **§12.3**: `production` requires two approvals, distinct from the requester — the environment was saved without its `approval`: the field does not exist, so nothing will wait
  - a publish creates a release request and the edge still serves the previous release — the publish answered 201 and moved the pointer at once: the edge serves the new release 268dfadc, not the previous fee66533 — no request was made and no approval waited
- **§12.4**: each runtime implements SPEC §1.4 (probe: the edge's manifests, a 10 % rollout under the table's salt) — runtimes without SPEC §1.4 rollout: js: 0 of 10000 ids activate the candidate and 1004 disagree with the generator — first `7a8d04dd33be8a887e8f4df849268a6b` (cohort 373) is on ef7970c1, want 3151eb1d; go: the runtime has no rollout surface — the driver does not compile: unknown field I…
  - start a rollout of the candidate at 10 % in project A's `production` — starting a rollout (POST /v1/tenants/01a0f6ad-7fd0-78f8-9bf2-8b200c36304c/projects/01a0f6ad-7fd5-7c5c-b2a3-137e5609fce4/environments/production/rollouts) does not exist on this server (HTTP 404: {"type":"urn:glossa:problem:not_found","title":"Not Found","status":404,"code":"not_found","detail":"no such resource"})
- **§12.5**: the tenant's audit entries can be listed — listing audit entries (GET /v1/tenants/01a0f6ad-7fd0-78f8-9bf2-8b200c36304c/audit-entries) does not exist on this server (HTTP 404: {"type":"urn:glossa:problem:not_found","title":"Not Found","status":404,"code":"not_found","detail":"no such resource"})
  - export the run's range as an audit export job — starting an audit export job (POST /v1/tenants/01a0f6ad-7fd0-78f8-9bf2-8b200c36304c/audit-export-jobs) does not exist on this server (HTTP 404: {"type":"urn:glossa:problem:not_found","title":"Not Found","status":404,"code":"not_found","detail":"no such resource"})
  - `glossa audit verify` passes on the export, offline — there is no export to verify, and `glossa audit verify` exits 2: error: unknown command "audit" /   fix:   run `glossa help` for the list of commands
- **§12.6**: publish, and render every key in every locale both ways: zero mismatches (imported by --v0-db) — 51 of 2400 renderings differ (first: `copy.bare_11` de map[name:Ada] — v0.3 "Gehts gut, {name}?", runtime "Geht's gut, Ada?")
  - the three users are invitations with mapped roles and locales — no matching invitation for admin@acme-v03.example (admin []), tomas@acme-v03.example (translator [en]), lucia@acme-v03.example (translator [es])
  - v0.3's history is visible as imported audit entries — listing audit entries does not exist on this server (HTTP 404: {"type":"urn:glossa:problem:not_found","title":"Not Found","status":404,"code":"not_found","detail":"no such resource"})

## The fixture

- **An organisation of three people and a vendor's translator**, all signed in through mail the server
  delivered over SMTP (`GLOSSA_MAIL_DRIVER=smtp`, with credentials) to the test mailer: the owner
  (`lena@acme.example`), two `de` reviewers (`ana@acme.example`, `ben@acme.example`), and the vendor's translator (`vera@lingua.example`), who registers
  with a password, follows the verification link the mailer received, and so accepts the invitation.
  The test mailer accepted 4 messages and refused 0 unauthenticated sessions.
- **Project A** (`ledger`) and **project B** (`portal`), source `en`, target `de`, 30 messages each plus
  `shared.welcome`, which both hold and §12.1 revises in both. Project B's first 20 `de` units are the vendor's
  assignment (§12.2); the rest, and all of project A, are outside it.
- The platform could not make the vendor's translator a vendor member (§12.2), so they were invited as an
  ordinary `de` translator: the sweep shows what such a member can read today.
- **Canaries**, words that exist only in this fixture's text, in source and translation text of both projects:
  `Quokkafjordine`, `Velutinaquill`, `Brombeerzwirnt`, `Nachtfalterzopf`, `Zwielichtmarmor`. §12.5 fails if any of them reaches the audit export.
- **A v0.3 server built from `apps/api`** (§12.6), migrated with its own migrations, seeded through its own API.

## §12.1 — two workflows, one event

| | Step | What happened |
|---|---|---|
| ❌ | refuse a definition naming a guard the vocabulary does not have with 422 `invalid_workflow` | saving a workflow definition (POST /v1/tenants/01a0f6ad-7fd0-78f8-9bf2-8b200c36304c/workflow-definitions) does not exist on this server (HTTP 404: {"type":"urn:glossa:problem:not_found","title":"Not Found","status":404,"code":"not_found","detail":"no such resource"}) |
| ❌ | refuse a definition with an unreachable state with 422 `invalid_workflow` | saving a workflow definition (POST /v1/tenants/01a0f6ad-7fd0-78f8-9bf2-8b200c36304c/workflow-definitions) does not exist on this server (HTTP 404: {"type":"urn:glossa:problem:not_found","title":"Not Found","status":404,"code":"not_found","detail":"no such resource"}) |
| ❌ | refuse a definition with a delayed transition with 422 `invalid_workflow` | saving a workflow definition (POST /v1/tenants/01a0f6ad-7fd0-78f8-9bf2-8b200c36304c/workflow-definitions) does not exist on this server (HTTP 404: {"type":"urn:glossa:problem:not_found","title":"Not Found","status":404,"code":"not_found","detail":"no such resource"}) |
| ❌ | §2.1's architecture test passes | `go test -run Architecture ./internal/workflow/...` ran no architecture test: ?   	github.com/felixgeelhaar/glossa/platform/internal/workflow/app	[no test files] / ?   	github.com/felixgeelhaar/glossa/platform/internal/workflow/defaults	[no test files] / ok  	github.com/felixgeelhaar/glossa/platform/internal/workflow/domain	0.247s [no tests to run] |
| ❌ | save `vendor-then-four-eyes` (testdata) through the API | saving a workflow definition (POST /v1/tenants/01a0f6ad-7fd0-78f8-9bf2-8b200c36304c/workflow-definitions) does not exist on this server (HTTP 404: {"type":"urn:glossa:problem:not_found","title":"Not Found","status":404,"code":"not_found","detail":"no such resource"}) |
| · | find the seeded default definition | _not reached_ |
| · | bind A and B | _not reached_ |
| · | revise the shared source | _not reached_ |
| · | project A's instance | _not reached_ |
| · | A: the `de` translation goes to `needs_review` | _not reached_ |
| · | A: one reviewer's approval makes it `approved` | _not reached_ |
| · | B: assigned to the vendor | _not reached_ |
| · | B: the vendor member writes and completes it | _not reached_ |
| · | B: the author's own approval is refused | _not reached_ |
| · | B: a token's approval is refused | _not reached_ |
| · | B: one reviewer is not enough | _not reached_ |
| · | B: two distinct reviewers make it `approved` | _not reached_ |
| · | both instances reached a final state, every action ran as a person, never as Workflow's principal | _not reached_ |

§2.1's architecture test: ?   	github.com/felixgeelhaar/glossa/platform/internal/workflow/app	[no test files] / ?   	github.com/felixgeelhaar/glossa/platform/internal/workflow/defaults	[no test files] / ok  	github.com/felixgeelhaar/glossa/platform/internal/workflow/domain	0.247s [no tests to run]

## §12.2 — vendor visibility on every surface

| | Step | What happened |
|---|---|---|
| ❌ | the vendor exists and its translator is a vendor member with visibility `assigned`, scoped to project B | the platform could not create a vendor (POST /v1/tenants/01a0f6ad-7fd0-78f8-9bf2-8b200c36304c/vendors); the member is an ordinary `de` translator |
| ❌ | assign 20 `de` units of project B to the vendor | creating an assignment (POST /v1/tenants/01a0f6ad-7fd0-78f8-9bf2-8b200c36304c/assignments) does not exist on this server (HTTP 404: {"type":"urn:glossa:problem:not_found","title":"Not Found","status":404,"code":"not_found","detail":"no such resource"}) |
| ❌ | writes outside the assignment are refused | the vendor member wrote `b.unit.21` and `a.unit.01`, outside the assignment |
| ❌ | export, import and TM search are refused | the vendor member was not refused: an export job (201), an import job (201), `GET /tm-concordance` (200) |

The sweep generated 99 GET operations from platform/api/openapi.yaml.

The vendor member could not create an API token (HTTP 403: {"type":"urn:glossa:problem:forbidden","title":"Forbidden","status":403,"code":"forbidden","detail":"missing permission tokens.manage"}), so no MCP tool is reachable as them.

### The coverage table

Every GET operation of `platform/api/openapi.yaml`, called as the vendor's translator — inside the
assignment (project B, an assigned unit) and, where the operation is addressed by a project or a
message, outside it (project A, an unassigned unit, which must answer 404). **29 hold, 57 show something
outside the assignment or answer undocumented, 13 have no verdict** (no fixture id to address them).

| | Operation | Inside | Outside | Why |
|---|---|---|---|---|
| ✅ | `getMe` | 200 | — |  |
| ✅ | `listPasskeys` | 200 | — |  |
| ✅ | `getMeta` | 200 | — |  |
| ✅ | `listTenants` | 200 | — |  |
| ✅ | `getTenant` | 200 | — |  |
| ✅ | `getAIBudget` | 200 | — |  |
| ✅ | `listAIDisclosures` | 200 | — |  |
| ✅ | `getAIEvalBaseline` | 200 | — |  |
| ∅ | `getAIFill` | — | — | no fixture id for `{ai_fill}` |
| ✅ | `listAIJobs` | 200 | — |  |
| ∅ | `getAIJob` | — | — | no fixture id for `{ai_job}` |
| ✅ | `getAIPrices` | 200 | — |  |
| ✅ | `listAIProviders` | 200 | — |  |
| ✅ | `getAIProvider` | 200 | — |  |
| ✅ | `getAIRoutingPolicy` | 200 | — |  |
| ✅ | `getAISettings` | 200 | — |  |
| ✅ | `listAISpend` | 200 | — |  |
| ✅ | `listAISuggestions` | 200 | — |  |
| ∅ | `getAISuggestion` | — | — | no fixture id for `{ai_suggestion}` |
| ✅ | `getEffectiveStyleGuide` | 200 | — |  |
| ❌ | `listExportJobs` | 200 | — | the answer holds `01a0f6ad-7fd5-7c5c-b2a3-137e5609fce4`, which is outside the assignment |
| ❌ | `getExportJob` | 200 | — | the answer holds `01a0f6ad-7fd5-7c5c-b2a3-137e5609fce4`, which is outside the assignment |
| ❌ | `downloadExportFile` | 200 | — | the answer holds `Quokkafjordine`, which is outside the assignment |
| ✅ | `listGitConnections` | 503 | — |  |
| ∅ | `getGitConnection` | — | — | no fixture id for `{connection}` |
| ✅ | `listGitHubInstallations` | 503 | — |  |
| ✅ | `listImportJobs` | 200 | — |  |
| ∅ | `getImportJob` | — | — | no fixture id for `{import_job}` |
| ∅ | `listImportResults` | — | — | no fixture id for `{import_job}` |
| ✅ | `listMembers` | 200 | — |  |
| ✅ | `getMember` | 200 | — |  |
| ❌ | `listProjects` | 200 | — | the answer holds `01a0f6ad-7fd5-7c5c-b2a3-137e5609fce4`, which is outside the assignment |
| ❌ | `getProject` | 200 | 200 | an id outside the assignment answered 200, want 404 (it holds `01a0f6ad-7fd5-7c5c-b2a3-137e5609fce4`) |
| ❌ | `getAIMetrics` | 200 | 200 | an id outside the assignment answered 200, want 404 |
| ❌ | `getAIReviewQueue` | 200 | 200 | an id outside the assignment answered 200, want 404 |
| ❌ | `getProjectAIRoutingPolicy` | 200 | 200 | an id outside the assignment answered 200, want 404 |
| ❌ | `getProjectAISettings` | 200 | 200 | an id outside the assignment answered 200, want 404 |
| ❌ | `listApplications` | 200 | 200 | an id outside the assignment answered 200, want 404 |
| ❌ | `getApplication` | 200 | 200 | an id outside the assignment answered 200, want 404 |
| ❌ | `listBranches` | 200 | 200 | an id outside the assignment answered 200, want 404 |
| ∅ | `getBranch` | — | — | no fixture id for `{branch}` |
| ∅ | `listBranchProposals` | — | — | no fixture id for `{branch}` |
| ∅ | `listCaptureFindings` | — | — | no fixture id for `{capture}` |
| ∅ | `getCaptureImage` | — | — | no fixture id for `{capture}` |
| ❌ | `getCheckPolicy` | 200 | 200 | an id outside the assignment answered 200, want 404 |
| ❌ | `exportCheckPolicy` | 200 | 200 | an id outside the assignment answered 200, want 404 |
| ❌ | `listCheckPolicyVersions` | 200 | 200 | an id outside the assignment answered 200, want 404 |
| ❌ | `getCheckPolicyVersion` | 200 | 200 | an id outside the assignment answered 200, want 404 |
| ❌ | `listCheckRuns` | 200 | 200 | an id outside the assignment answered 200, want 404 |
| ∅ | `getCheckRun` | — | — | no fixture id for `{check_run}` |
| ❌ | `listContextBuilds` | 200 | 200 | an id outside the assignment answered 200, want 404 |
| ❌ | `listDeliveryKeys` | 200 | 200 | an id outside the assignment answered 200, want 404 |
| ❌ | `listEnvironments` | 200 | 200 | an id outside the assignment answered 200, want 404 |
| ❌ | `getEnvironment` | 200 | 200 | an id outside the assignment answered 200, want 404 |
| ❌ | `listDeployments` | 200 | 200 | an id outside the assignment answered 200, want 404 |
| ❌ | `getFallbackGraph` | 200 | 200 | an id outside the assignment answered 200, want 404 |
| ❌ | `listFindings` | 200 | 200 | an id outside the assignment answered 200, want 404 |
| ❌ | `listLinguisticJobs` | 200 | 200 | an id outside the assignment answered 200, want 404 |
| ∅ | `getLinguisticJob` | — | — | no fixture id for `{linguistic_job}` |
| ❌ | `listLocales` | 200 | 200 | an id outside the assignment answered 200, want 404 |
| ❌ | `getLocale` | 200 | 200 | an id outside the assignment answered 200, want 404 |
| ❌ | `listMessages` | 200 | 200 | the answer holds `b.unit.21`, which is outside the assignment |
| ❌ | `getMessage` | 200 | 200, 200 | an id outside the assignment answered 200, want 404 (it holds `of the a screen`) |
| ❌ | `listMessageCaptures` | 200 | 200, 200 | an id outside the assignment answered 200, want 404 (it holds `a.unit.01`) |
| ❌ | `listSourceRevisions` | 200 | 200, 200 | an id outside the assignment answered 200, want 404 (it holds `of the a screen`) |
| ❌ | `listMessageTranslations` | 200 | 200, 200 | an id outside the assignment answered 200, want 404 (it holds `a.unit.01`) |
| ❌ | `getTranslation` | 200 | 200, 200 | an id outside the assignment answered 200, want 404 (it holds `a.unit.01`) |
| ❌ | `listTranslationRevisions` | 200 | 200, 200 | an id outside the assignment answered 200, want 404 (it holds `a.unit.01`) |
| ❌ | `listMessageUsages` | 200 | 200, 200 | an id outside the assignment answered 200, want 404 (it holds `a.unit.01`) |
| ❌ | `listNamespaces` | 200 | 200 | an id outside the assignment answered 200, want 404 |
| ❌ | `listPreviewOrigins` | 200 | 200 | an id outside the assignment answered 200, want 404 |
| ❌ | `getQualitySummary` | 200 | 200 | an id outside the assignment answered 200, want 404 (it holds `01a0f6ad-7fd5-7c5c-b2a3-137e5609fce4`) |
| ❌ | `listReleaseSigningKeys` | 200 | 200 | an id outside the assignment answered 200, want 404 |
| ❌ | `listReleases` | 200 | 200 | an id outside the assignment answered 200, want 404 |
| ❌ | `getRelease` | 200 | 200 | an id outside the assignment answered 200, want 404 |
| ∅ | `getReleaseArtifact` | — | — | no fixture id for `{digest}` |
| ❌ | `getReleaseDiff` | 200 | 200 | the answer holds `b.unit.21`, which is outside the assignment |
| ❌ | `getReleaseManifest` | 400 | 400 | an id outside the assignment answered 400, want 404 |
| ❌ | `listProjectTerminologyFindings` | 400 | 400 | an id outside the assignment answered 400, want 404 |
| ❌ | `getTranslationStats` | 200 | 200 | an id outside the assignment answered 200, want 404 |
| ❌ | `listProjectTranslations` | 400 | 400 | an id outside the assignment answered 400, want 404 |
| ❌ | `listUnusedMessages` | 200 | 200 | the answer holds `b.unit.21`, which is outside the assignment |
| ❌ | `listUsages` | 200 | 200 | an id outside the assignment answered 200, want 404 |
| ❌ | `listWaivers` | 200 | 200 | an id outside the assignment answered 200, want 404 |
| ❌ | `listStyleGuides` | 200 | — | the answer holds `01a0f6ad-7fd5-7c5c-b2a3-137e5609fce4`, which is outside the assignment |
| ❌ | `getStyleGuide` | 200 | — | the answer holds `01a0f6ad-7fd5-7c5c-b2a3-137e5609fce4`, which is outside the assignment |
| ❌ | `listStyleGuideVersions` | 200 | — | the answer holds `01a0f6ad-7fd5-7c5c-b2a3-137e5609fce4`, which is outside the assignment |
| ❌ | `listTermConcepts` | 200 | — | the answer holds `01a0f6ad-7fd5-7c5c-b2a3-137e5609fce4`, which is outside the assignment |
| ❌ | `getTermConcept` | 200 | — | the answer holds `01a0f6ad-7fd5-7c5c-b2a3-137e5609fce4`, which is outside the assignment |
| ❌ | `listTermConceptRevisions` | 200 | — | the answer holds `01a0f6ad-7fd5-7c5c-b2a3-137e5609fce4`, which is outside the assignment |
| ✅ | `listTermbaseExportJobs` | 200 | — |  |
| ✅ | `listTermbaseImportJobs` | 200 | — |  |
| ✅ | `searchTranslationMemory` | 400 | — |  |
| ✅ | `listTMExportJobs` | 200 | — |  |
| ✅ | `listTMImportJobs` | 200 | — |  |
| ❌ | `listTranslationMemoryUnits` | 200 | — | the answer holds `01a0f6ad-7fd5-7c5c-b2a3-137e5609fce4`, which is outside the assignment |
| ❌ | `getTranslationMemoryUnit` | 200 | — | the answer holds `01a0f6ad-7fd5-7c5c-b2a3-137e5609fce4`, which is outside the assignment |
| ✅ | `listTokens` | 403 | — |  |
| ✅ | `getToken` | 403 | — |  |

The MCP read tools, as the same member:

| | Tool | Answer | Why |
|---|---|---|---|
| ✅ | `(connect)` | no token | refused at token creation |

## §12.3 — release approvals

| | Step | What happened |
|---|---|---|
| ❌ | `production` requires two approvals, distinct from the requester | the environment was saved without its `approval`: the field does not exist, so nothing will wait |
| ❌ | a publish creates a release request and the edge still serves the previous release | the publish answered 201 and moved the pointer at once: the edge serves the new release 268dfadc, not the previous fee66533 — no request was made and no approval waited |
| · | the requester's own approval is refused | _not reached_ |
| · | after the first approval the edge still serves the previous release | _not reached_ |
| · | after the second approval the edge serves the new release | _not reached_ |
| · | a forced publish over an unmet gate still waits for approvals, and the approvers see its reason | _not reached_ |
| ✅ | a rollback takes effect at the edge at once, with no approval | held |

What `glossa-edge` served for project B's `production`, read over HTTP from the edge process:

| | When | The edge served |
|---|---|---|
| ✅ | before approvals are required | v2 |
| ❌ | right after the publish | the new release 268dfadc |
| ✅ | after a rollback, no approval | the earlier release, within 5.1s |

## §12.4 — staged rollout across three runtimes

| | Step | What happened |
|---|---|---|
| ✅ | the generator's cohort table: 10,000 installation ids with their SPEC §1.4 cohorts | held |
| ❌ | each runtime implements SPEC §1.4 (probe: the edge's manifests, a 10 % rollout under the table's salt) | runtimes without SPEC §1.4 rollout: js: 0 of 10000 ids activate the candidate and 1004 disagree with the generator — first `7a8d04dd33be8a887e8f4df849268a6b` (cohort 373) is on ef7970c1, want 3151eb1d; go: the runtime has no rollout surface — the driver does not compile: unknown field InstallationID in struct literal of type glossa.Config; unknown field DisableRollout in struct literal of typ… |
| ❌ | start a rollout of the candidate at 10 % in project A's `production` | starting a rollout (POST /v1/tenants/01a0f6ad-7fd0-78f8-9bf2-8b200c36304c/projects/01a0f6ad-7fd5-7c5c-b2a3-137e5609fce4/environments/production/rollouts) does not exist on this server (HTTP 404: {"type":"urn:glossa:problem:not_found","title":"Not Found","status":404,"code":"not_found","detail":"no such resource"}) |
| · | the edge's manifest carries it | _not reached_ |
| · | three runtimes at 10 %, against the generator | _not reached_ |
| · | rollout support off | _not reached_ |
| · | advance to 50 % keeps every 10 % installation | _not reached_ |
| · | abort returns all of them to stable | _not reached_ |
| · | complete moves the pointer | _not reached_ |

The generator's table (`runtimes/testdata/rollout/cohorts.json`): 10000 installation ids; under its salt `c3RhZ2VkLXJvbGxvdXQtMQ`, 1004 are in the candidate at 10 %.

| | Phase | Runtime | In the candidate | Disagree with the generator | Note |
|---|---|---|---:|---:|---|
| ❌ | probe at 10 % | js | 0 | 1004 | `7a8d04dd33be8a887e8f4df849268a6b` (cohort 373) is on ef7970c1, want 3151eb1d |
| ❌ | probe at 10 % | go | 0 | 0 | the runtime has no rollout surface — the driver does not compile: unknown field InstallationID in struct literal of type glossa.Config; unknown field DisableRollout in struct literal of type glossa.Config |
| ❌ | probe at 10 % | dart | 0 | 0 | the runtime has no rollout surface — the driver does not compile: No named parameter with the name 'installationId'. |
| ✅ | probe, rollout support off | js | 0 | 0 |  |
| ❌ | probe, rollout support off | go | 0 | 0 | the runtime has no rollout surface — the driver does not compile: unknown field InstallationID in struct literal of type glossa.Config; unknown field DisableRollout in struct literal of type glossa.Config |
| ❌ | probe, rollout support off | dart | 0 | 0 | the runtime has no rollout surface — the driver does not compile: No named parameter with the name 'installationId'. |

## §12.5 — audit export

| | Step | What happened |
|---|---|---|
| ❌ | the tenant's audit entries can be listed | listing audit entries (GET /v1/tenants/01a0f6ad-7fd0-78f8-9bf2-8b200c36304c/audit-entries) does not exist on this server (HTTP 404: {"type":"urn:glossa:problem:not_found","title":"Not Found","status":404,"code":"not_found","detail":"no such resource"}) |
| ❌ | export the run's range as an audit export job | starting an audit export job (POST /v1/tenants/01a0f6ad-7fd0-78f8-9bf2-8b200c36304c/audit-export-jobs) does not exist on this server (HTTP 404: {"type":"urn:glossa:problem:not_found","title":"Not Found","status":404,"code":"not_found","detail":"no such resource"}) |
| ❌ | `glossa audit verify` passes on the export, offline | there is no export to verify, and `glossa audit verify` exits 2: error: unknown command "audit" /   fix:   run `glossa help` for the list of commands |
| · | an entry for every call the harness recorded, with its actor | _not reached_ |
| · | one altered byte makes `glossa audit verify` fail | _not reached_ |
| · | no canary string appears anywhere in the export | _not reached_ |

The harness recorded 14 successful mutating calls in §12.1–§12.4 itself, never from the platform: map[12.2:4 12.3:6 12.4:4].

| § | Successful mutating calls the harness recorded |
|---|---:|
| 12.2 | 4 |
| 12.3 | 6 |
| 12.4 | 4 |

## §12.6 — v0.3 imports and renders the same

| | Step | What happened |
|---|---|---|
| ✅ | a v0.3 server built from apps/api, migrated with its own migrations, seeded: 300 keys in de/en/es, descriptions, a change history, three users | held |
| ✅ | dump it and restore the dump with `platform/scripts/v0-restore.sh`, which writes the restore marker | held |
| ✅ | `glossa import --from v0 --v0-db` imports the restore | held |
| ❌ | publish, and render every key in every locale both ways: zero mismatches (imported by --v0-db) | 51 of 2400 renderings differ (first: `copy.bare_11` de map[name:Ada] — v0.3 "Gehts gut, {name}?", runtime "Geht's gut, Ada?") |
| ✅ | descriptions are on the messages | held |
| ❌ | the three users are invitations with mapped roles and locales | no matching invitation for admin@acme-v03.example (admin []), tomas@acme-v03.example (translator [en]), lucia@acme-v03.example (translator [es]) |
| ❌ | v0.3's history is visible as imported audit entries | listing audit entries does not exist on this server (HTTP 404: {"type":"urn:glossa:problem:not_found","title":"Not Found","status":404,"code":"not_found","detail":"no such resource"}) |
| ✅ | the importer refuses a DSN without the restore marker | held |

v0.3 holds 300 keys × 3 locales, 100 audit-log rows and 3 users.

Imported by --v0-db; **2400 renderings** of 300 keys in de/en/es, **51 differ**.

| Key | Locale | Arguments | v0.3's formatter | @glossa/runtime |
|---|---|---|---|---|
| `copy.bare_11` | de | `map[name:Ada]` | "Gehts gut, {name}?" | "Geht's gut, Ada?" |
| `copy.bare_11` | en | `map[name:Ada]` | "Dont wait, {name}" | "Don't wait, Ada" |
| `copy.bare_11` | es | `map[name:Ada]` | "Rocknroll con Ada" | "Rock'n'roll con Ada" |
| `copy.bare_29` | de | `map[name:Ada]` | "Gehts gut, {name}?" | "Geht's gut, Ada?" |
| `copy.bare_29` | en | `map[name:Ada]` | "Dont wait, {name}" | "Don't wait, Ada" |
| `copy.bare_29` | es | `map[name:Ada]` | "Rocknroll con Ada" | "Rock'n'roll con Ada" |
| `copy.bare_47` | de | `map[name:Ada]` | "Gehts gut, {name}?" | "Geht's gut, Ada?" |
| `copy.bare_47` | en | `map[name:Ada]` | "Dont wait, {name}" | "Don't wait, Ada" |
| `copy.bare_47` | es | `map[name:Ada]` | "Rocknroll con Ada" | "Rock'n'roll con Ada" |
| `copy.bare_65` | de | `map[name:Ada]` | "Gehts gut, {name}?" | "Geht's gut, Ada?" |
| `copy.bare_65` | en | `map[name:Ada]` | "Dont wait, {name}" | "Don't wait, Ada" |
| `copy.bare_65` | es | `map[name:Ada]` | "Rocknroll con Ada" | "Rock'n'roll con Ada" |
| `copy.bare_83` | de | `map[name:Ada]` | "Gehts gut, {name}?" | "Geht's gut, Ada?" |
| `copy.bare_83` | en | `map[name:Ada]` | "Dont wait, {name}" | "Don't wait, Ada" |
| `copy.bare_83` | es | `map[name:Ada]` | "Rocknroll con Ada" | "Rock'n'roll con Ada" |
| `copy.bare_101` | de | `map[name:Ada]` | "Gehts gut, {name}?" | "Geht's gut, Ada?" |
| `copy.bare_101` | en | `map[name:Ada]` | "Dont wait, {name}" | "Don't wait, Ada" |
| `copy.bare_101` | es | `map[name:Ada]` | "Rocknroll con Ada" | "Rock'n'roll con Ada" |
| `copy.bare_119` | de | `map[name:Ada]` | "Gehts gut, {name}?" | "Geht's gut, Ada?" |
| `copy.bare_119` | en | `map[name:Ada]` | "Dont wait, {name}" | "Don't wait, Ada" |
| `copy.bare_119` | es | `map[name:Ada]` | "Rocknroll con Ada" | "Rock'n'roll con Ada" |
| `copy.bare_137` | de | `map[name:Ada]` | "Gehts gut, {name}?" | "Geht's gut, Ada?" |
| `copy.bare_137` | en | `map[name:Ada]` | "Dont wait, {name}" | "Don't wait, Ada" |
| `copy.bare_137` | es | `map[name:Ada]` | "Rocknroll con Ada" | "Rock'n'roll con Ada" |
| `copy.bare_155` | de | `map[name:Ada]` | "Gehts gut, {name}?" | "Geht's gut, Ada?" |
| `copy.bare_155` | en | `map[name:Ada]` | "Dont wait, {name}" | "Don't wait, Ada" |
| `copy.bare_155` | es | `map[name:Ada]` | "Rocknroll con Ada" | "Rock'n'roll con Ada" |
| `copy.bare_173` | de | `map[name:Ada]` | "Gehts gut, {name}?" | "Geht's gut, Ada?" |
| `copy.bare_173` | en | `map[name:Ada]` | "Dont wait, {name}" | "Don't wait, Ada" |
| `copy.bare_173` | es | `map[name:Ada]` | "Rocknroll con Ada" | "Rock'n'roll con Ada" |
| `copy.bare_191` | de | `map[name:Ada]` | "Gehts gut, {name}?" | "Geht's gut, Ada?" |
| `copy.bare_191` | en | `map[name:Ada]` | "Dont wait, {name}" | "Don't wait, Ada" |
| `copy.bare_191` | es | `map[name:Ada]` | "Rocknroll con Ada" | "Rock'n'roll con Ada" |
| `copy.bare_209` | de | `map[name:Ada]` | "Gehts gut, {name}?" | "Geht's gut, Ada?" |
| `copy.bare_209` | en | `map[name:Ada]` | "Dont wait, {name}" | "Don't wait, Ada" |
| `copy.bare_209` | es | `map[name:Ada]` | "Rocknroll con Ada" | "Rock'n'roll con Ada" |
| `copy.bare_227` | de | `map[name:Ada]` | "Gehts gut, {name}?" | "Geht's gut, Ada?" |
| `copy.bare_227` | en | `map[name:Ada]` | "Dont wait, {name}" | "Don't wait, Ada" |
| `copy.bare_227` | es | `map[name:Ada]` | "Rocknroll con Ada" | "Rock'n'roll con Ada" |
| `copy.bare_245` | de | `map[name:Ada]` | "Gehts gut, {name}?" | "Geht's gut, Ada?" |
| … | | | 11 more | |

## §12.7 — earlier exits hold

M2's exit test passed in 55s.

M3's exit test passed in 89s.

M4's exit test passed in 308s.

### M2 — **passed** in 55s

```text
--- PASS: TestM2Exit (53.42s)
--- PASS: TestFixtureIsCurrent (0.11s)
--- PASS: TestFixtureShape (0.00s)
--- PASS: TestInterchangeFilesRead (0.03s)
ok  	github.com/felixgeelhaar/glossa/platform/internal/systemtest/m2	53.835s
ok  	github.com/felixgeelhaar/glossa/platform/internal/systemtest/m2/fixture	0.356s
```

### M3 — **passed** in 89s

```text
--- PASS: TestM3Exit (82.43s)
--- PASS: TestFixtureIsCurrent (0.03s)
--- PASS: TestFixtureShape (0.01s)
ok  	github.com/felixgeelhaar/glossa/platform/internal/systemtest/m3	83.562s
ok  	github.com/felixgeelhaar/glossa/platform/internal/systemtest/m3/fixture	0.472s
```

### M4 — **passed** in 308s

```text
--- PASS: TestM4Exit (305.08s)
ok  	github.com/felixgeelhaar/glossa/platform/internal/systemtest/m4	305.456s
**8 of the 8 exit criteria hold.**
| § | Criterion | Verdict |
|---|---|---|
| 12.1 | A fixture repository with real CI | met |
| 12.2 | Findings across layers | met |
| 12.3 | The PR check agrees | met |
| 12.4 | Waivers and policy rollout | met |
| 12.5 | Release gate | met |
| 12.6 | MCP | met |
| 12.7 | Flutter | met |
| 12.8 | Dashboard | met |
```

## What this test cannot prove

RFC 0006 §1.3 and §12: production rendering in real products, a real vendor's onboarding without a test
mailer, real cohort proportions, and v0.3's shutdown. Those are the dogfood phase (§7.4).
