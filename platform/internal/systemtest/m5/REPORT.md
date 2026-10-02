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

**3 of the 7 exit criteria hold.**

| § | Criterion | Verdict | Fails if |
|---|---|---|---|
| 12.1 | Two workflows, one event | met | B needs a code path A doesn't; an instance strands short of a final state; the author's or a token's grant counts; Workflow's principal approves anything. |
| 12.2 | Vendor visibility on every surface | met | any generated operation leaks an id outside the assignment, or an operation in the spec has no verdict in the coverage table. |
| 12.3 | Release approvals | **not met** | any pointer moves before the second approval as seen at the edge, or a rollback waits. |
| 12.4 | Staged rollout across three runtimes | **not met** | any runtime disagrees with the generator on any id, the share is outside 9–11 %, or an aborted installation stays on the candidate. Runtimes are compared with the generator, never with each other. |
| 12.5 | Audit export | **not met** | a call the harness recorded has no entry (compared with the harness's own log, not the outbox), an entry has the wrong actor, a tampered export verifies, or a canary leaks. |
| 12.6 | v0.3 imports and renders the same | **not met** | any rendering differs between v0.3's formatter and @glossa/runtime (two implementations that share no code), or a carried field is missing. |
| 12.7 | Earlier exits hold | met | any earlier exit criterion fails. A failure here blocks the M5 verdict whatever 12.1–12.6 say. |

What is missing, in one line each:

- **§12.3**: `production` requires two approvals, distinct from the requester — the environment was saved without its `approval`: the field does not exist, so nothing will wait
  - a publish creates a release request and the edge still serves the previous release — the publish answered 201 and moved the pointer at once: the edge serves the new release ed8b7288, not the previous 1a69b891 — no request was made and no approval waited
- **§12.4**: start a rollout of the candidate at 10 % in project A's `production` — starting a rollout (POST /v1/tenants/01a0fb69-df6e-74e2-ab53-2ddee7731b6e/projects/01a0fb69-df72-7595-8c1a-70554b2e3530/environments/production/rollouts) does not exist on this server (HTTP 404: {"type":"urn:glossa:problem:not_found","title":"Not Found","status":404,"code":"not_found","detail":"no such resource"})
- **§12.5**: the tenant's audit entries can be listed — listing audit entries (GET /v1/tenants/01a0fb69-df6e-74e2-ab53-2ddee7731b6e/audit-entries) does not exist on this server (HTTP 404: {"type":"urn:glossa:problem:not_found","title":"Not Found","status":404,"code":"not_found","detail":"no such resource"})
  - export the run's range as an audit export job — starting an audit export job (POST /v1/tenants/01a0fb69-df6e-74e2-ab53-2ddee7731b6e/audit-export-jobs) does not exist on this server (HTTP 404: {"type":"urn:glossa:problem:not_found","title":"Not Found","status":404,"code":"not_found","detail":"no such resource"})
  - `glossa audit verify` passes on the export, offline — there is no export to verify, and `glossa audit verify` exits 2: error: unknown command "audit" /   fix:   run `glossa help` for the list of commands
- **§12.6**: publish, and render every key in every locale both ways: zero mismatches (imported by --v0-db) — 51 of 2400 renderings differ (first: `copy.bare_11` de map[name:Ada] — v0.3 "Gehts gut, {name}?", runtime "Geht's gut, Ada?")
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
- The vendor's translator is a vendor member with visibility `assigned`, scoped to project B.
- **Something to address in each project** (§12.2), made through the API as the owner: an application,
  a check policy, a staging release and one of its artifacts, an import job, a Git connection (one
  repository of a fake GitHub, under a path per project), a branch push, a capture upload, a check run,
  a linguistic job, and an AI fill of `a.unit.19` / `b.unit.19` with its job and suggestion, drafted by a fake
  provider on loopback. Project B's are the sweep's ids inside the assignment, project A's outside it.
- **Canaries**, words that exist only in this fixture's text, in source and translation text of both projects:
  `Quokkafjordine`, `Velutinaquill`, `Brombeerzwirnt`, `Nachtfalterzopf`, `Zwielichtmarmor`. §12.5 fails if any of them reaches the audit export.
- **A v0.3 server built from `apps/api`** (§12.6), migrated with its own migrations, seeded through its own API.

## §12.1 — two workflows, one event

| | Step | What happened |
|---|---|---|
| ✅ | refuse a definition naming a guard the vocabulary does not have with 422 `invalid_workflow` | held |
| ✅ | refuse a definition with an unreachable state with 422 `invalid_workflow` | held |
| ✅ | refuse a definition with a delayed transition with 422 `invalid_workflow` | held |
| ✅ | §2.1's architecture test passes | held |
| ✅ | save `vendor-then-four-eyes` (testdata) through the API | held |
| ✅ | find the seeded default definition (`defaults/review.json`) | held |
| ✅ | bind project A to the default and project B to `vendor-then-four-eyes` for `de` | held |
| ✅ | revise `shared.welcome`'s source in both projects to the same revision | held |
| ✅ | A: an instance of the default definition exists and the `de` translation is `needs_review` | held |
| ✅ | A: one reviewer's approval makes it `approved`, by that reviewer in the revision log | held |
| ✅ | B: the `de` unit is assigned to the vendor and appears in the vendor member's work | held |
| ✅ | B: the vendor member writes it and completes the assignment | held |
| ✅ | B: the author's own approval is refused | held |
| ✅ | B: a token's approval is refused (no scope grants `approvals.decide`) | held |
| ✅ | B: after one reviewer the translation is still not `approved` | held |
| ✅ | B: the second reviewer's approval makes it `approved`, by that reviewer in the revision log | held |
| ✅ | both instances reached a final state, every action ran as a person, never as Workflow's principal | held |

§2.1's architecture test: --- PASS: TestTheVocabularyIsClosed (0.00s) / PASS / ok  	github.com/felixgeelhaar/glossa/platform/internal/workflow/domain	0.270s

The transition logs:

```text
ad11e012:  —translation.outdated→ reviewing (person:01a0fb69-df5f-7a68-96da-525ef707fc3d)
ad11e012: reviewing —translation.reviewed→ reviewing (person:01a0fb69-df5f-7a68-96da-525ef707fc3d)
ad11e012: reviewing —approval.granted→ done (person:01a0fb69-df8c-72bb-a278-358bfd31070a)
474f92ea:  —translation.outdated→ translating (person:01a0fb69-df5f-7a68-96da-525ef707fc3d)
474f92ea: translating —translation.revised→ translating (person:01a0fb69-e06c-7451-b239-ea36d414e8ee)
474f92ea: translating —assignment.completed→ reviewing (person:01a0fb69-e06c-7451-b239-ea36d414e8ee)
474f92ea: reviewing —approval.granted→ reviewing (person:01a0fb69-df8c-72bb-a278-358bfd31070a)
474f92ea: reviewing —approval.granted→ done (person:01a0fb69-df9e-7900-bba0-8ea00ffe4f36)
```

## §12.2 — vendor visibility on every surface

| | Step | What happened |
|---|---|---|
| ✅ | the vendor exists and its translator is a vendor member with visibility `assigned`, scoped to project B | held |
| ✅ | assign 20 `de` units of project B to the vendor | held |
| ✅ | writes outside the assignment are refused | held |
| ✅ | export, import and TM search are refused | held |

The sweep generated 116 GET operations from platform/api/openapi.yaml.

The vendor member could not create an API token (HTTP 403: {"type":"urn:glossa:problem:forbidden","title":"Forbidden","status":403,"code":"forbidden","detail":"missing permission tokens.manage"}), so no MCP tool is reachable as them.

### The coverage table

Every GET operation of `platform/api/openapi.yaml`, called as the vendor's translator — inside the
assignment (project B, an assigned unit) and, where the operation is addressed by a project or a
message, outside it (project A, an unassigned unit, which must answer 404). **116 hold, 0 show something
outside the assignment or answer undocumented, 0 have no verdict** (no fixture id to address them).

| | Operation | Inside | Outside | Why |
|---|---|---|---|---|
| ✅ | `getMe` | 200 | — |  |
| ✅ | `listPasskeys` | 200 | — |  |
| ✅ | `getMeta` | 200 | — |  |
| ✅ | `listTenants` | 200 | — |  |
| ✅ | `getTenant` | 200 | — |  |
| ✅ | `getAIBudget` | 403 | — |  |
| ✅ | `listAIDisclosures` | 403 | — |  |
| ✅ | `getAIEvalBaseline` | 403 | — |  |
| ✅ | `getAIFill` | 403 | 404 |  |
| ✅ | `listAIJobs` | 403 | — |  |
| ✅ | `getAIJob` | 403 | 404 |  |
| ✅ | `getAIPrices` | 403 | — |  |
| ✅ | `listAIProviders` | 403 | — |  |
| ✅ | `getAIProvider` | 403 | — |  |
| ✅ | `getAIRoutingPolicy` | 403 | — |  |
| ✅ | `getAISettings` | 403 | — |  |
| ✅ | `listAISpend` | 403 | — |  |
| ✅ | `listAISuggestions` | 403 | — |  |
| ✅ | `getAISuggestion` | 403 | 404 |  |
| ✅ | `listApprovals` | 403 | — |  |
| ✅ | `getApproval` | 403 | — |  |
| ✅ | `listAssignments` | 200 | — |  |
| ✅ | `getAssignment` | 200 | — |  |
| ✅ | `getEffectiveStyleGuide` | 403 | — |  |
| ✅ | `listExportJobs` | 403 | — |  |
| ✅ | `getExportJob` | 404 | — |  |
| ✅ | `downloadExportFile` | 404 | — |  |
| ✅ | `listGitConnections` | 403 | — |  |
| ✅ | `getGitConnection` | 403 | 404 |  |
| ✅ | `listGitHubInstallations` | 403 | — |  |
| ✅ | `listGroups` | 403 | — |  |
| ✅ | `getGroup` | 403 | — |  |
| ✅ | `listImportJobs` | 403 | — |  |
| ✅ | `getImportJob` | 403 | 404 |  |
| ✅ | `listImportResults` | 403 | 404 |  |
| ✅ | `listMembers` | 403 | — |  |
| ✅ | `getMember` | 403 | — |  |
| ✅ | `listProjects` | 200 | — |  |
| ✅ | `getProject` | 200 | 404 |  |
| ✅ | `getAIMetrics` | 403 | 404 |  |
| ✅ | `getAIReviewQueue` | 403 | 404 |  |
| ✅ | `getProjectAIRoutingPolicy` | 403 | 404 |  |
| ✅ | `getProjectAISettings` | 403 | 404 |  |
| ✅ | `listApplications` | 403 | 404 |  |
| ✅ | `getApplication` | 403 | 404 |  |
| ✅ | `listBranches` | 403 | 404 |  |
| ✅ | `getBranch` | 403 | 404 |  |
| ✅ | `listBranchProposals` | 403 | 404 |  |
| ✅ | `listCaptureFindings` | 403 | 404 |  |
| ✅ | `getCaptureImage` | 404 | 404 |  |
| ✅ | `getCheckPolicy` | 403 | 404 |  |
| ✅ | `exportCheckPolicy` | 403 | 404 |  |
| ✅ | `listCheckPolicyVersions` | 403 | 404 |  |
| ✅ | `getCheckPolicyVersion` | 403 | 404 |  |
| ✅ | `listCheckRuns` | 403 | 404 |  |
| ✅ | `getCheckRun` | 403 | 404 |  |
| ✅ | `listContextBuilds` | 403 | 404 |  |
| ✅ | `listDeliveryKeys` | 403 | 404 |  |
| ✅ | `listEnvironments` | 403 | 404 |  |
| ✅ | `getEnvironment` | 403 | 404 |  |
| ✅ | `listDeployments` | 403 | 404 |  |
| ✅ | `getFallbackGraph` | 403 | 404 |  |
| ✅ | `listFindings` | 403 | 404 |  |
| ✅ | `listLinguisticJobs` | 403 | 404 |  |
| ✅ | `getLinguisticJob` | 403 | 404 |  |
| ✅ | `listLocales` | 200 | 404 |  |
| ✅ | `getLocale` | 200 | 404 |  |
| ✅ | `listMessages` | 200 | 404 |  |
| ✅ | `getMessage` | 200 | 404, 404 |  |
| ✅ | `listMessageCaptures` | 200 | 404, 404 |  |
| ✅ | `listSourceRevisions` | 200 | 404, 404 |  |
| ✅ | `listMessageTranslations` | 200 | 404, 404 |  |
| ✅ | `getTranslation` | 200 | 404, 404 |  |
| ✅ | `listTranslationRevisions` | 200 | 404, 404 |  |
| ✅ | `listMessageUsages` | 200 | 404, 404 |  |
| ✅ | `listNamespaces` | 403 | 404 |  |
| ✅ | `listPreviewOrigins` | 403 | 404 |  |
| ✅ | `getQualitySummary` | 403 | 404 |  |
| ✅ | `listReleaseSigningKeys` | 403 | 404 |  |
| ✅ | `listReleases` | 403 | 404 |  |
| ✅ | `getRelease` | 403 | 404 |  |
| ✅ | `getReleaseArtifact` | 403 | 404 |  |
| ✅ | `getReleaseDiff` | 403 | 404 |  |
| ✅ | `getReleaseManifest` | 403 | 404 |  |
| ✅ | `listProjectTerminologyFindings` | 403 | 404 |  |
| ✅ | `getTranslationStats` | 403 | 404 |  |
| ✅ | `listProjectTranslations` | 200 | 404 |  |
| ✅ | `listUnusedMessages` | 403 | 404 |  |
| ✅ | `listUsages` | 403 | 404 |  |
| ✅ | `listWaivers` | 403 | 404 |  |
| ✅ | `listWorkflowBindings` | 403 | 404 |  |
| ✅ | `listWorkflowInstances` | 403 | 404 |  |
| ✅ | `getWorkflowInstance` | 403 | 404 |  |
| ✅ | `listWorkflowTransitions` | 403 | 404 |  |
| ✅ | `resolveWorkflow` | 403 | 404 |  |
| ✅ | `listStyleGuides` | 403 | — |  |
| ✅ | `getStyleGuide` | 403 | — |  |
| ✅ | `listStyleGuideVersions` | 403 | — |  |
| ✅ | `listTermConcepts` | 403 | — |  |
| ✅ | `getTermConcept` | 403 | — |  |
| ✅ | `listTermConceptRevisions` | 403 | — |  |
| ✅ | `listTermbaseExportJobs` | 403 | — |  |
| ✅ | `listTermbaseImportJobs` | 403 | — |  |
| ✅ | `searchTranslationMemory` | 403 | — |  |
| ✅ | `listTMExportJobs` | 403 | — |  |
| ✅ | `listTMImportJobs` | 403 | — |  |
| ✅ | `listTranslationMemoryUnits` | 403 | — |  |
| ✅ | `getTranslationMemoryUnit` | 403 | — |  |
| ✅ | `listTokens` | 403 | — |  |
| ✅ | `getToken` | 403 | — |  |
| ✅ | `listVendors` | 403 | — |  |
| ✅ | `getVendor` | 403 | — |  |
| ✅ | `listWorkflowDefinitions` | 403 | — |  |
| ✅ | `getWorkflowDefinition` | 403 | — |  |
| ✅ | `listWorkflowDefinitionVersions` | 403 | — |  |
| ✅ | `getWorkflowDefinitionVersion` | 403 | — |  |

The MCP read tools, as the same member:

| | Tool | Answer | Why |
|---|---|---|---|
| ✅ | `(connect)` | no token | refused at token creation |

## §12.3 — release approvals

| | Step | What happened |
|---|---|---|
| ❌ | `production` requires two approvals, distinct from the requester | the environment was saved without its `approval`: the field does not exist, so nothing will wait |
| ❌ | a publish creates a release request and the edge still serves the previous release | the publish answered 201 and moved the pointer at once: the edge serves the new release ed8b7288, not the previous 1a69b891 — no request was made and no approval waited |
| · | the requester's own approval is refused | _not reached_ |
| · | after the first approval the edge still serves the previous release | _not reached_ |
| · | after the second approval the edge serves the new release | _not reached_ |
| · | a forced publish over an unmet gate still waits for approvals, and the approvers see its reason | _not reached_ |
| ✅ | a rollback takes effect at the edge at once, with no approval | held |

What `glossa-edge` served for project B's `production`, read over HTTP from the edge process:

| | When | The edge served |
|---|---|---|
| ✅ | before approvals are required | v2 |
| ❌ | right after the publish | the new release ed8b7288 |
| ✅ | after a rollback, no approval | the earlier release, within 5.1s |

## §12.4 — staged rollout across three runtimes

| | Step | What happened |
|---|---|---|
| ✅ | the generator's cohort table: 10,000 installation ids with their SPEC §1.4 cohorts | held |
| ✅ | each runtime implements SPEC §1.4 (probe: the edge's manifests, a 10 % rollout under the table's salt) | held |
| ❌ | start a rollout of the candidate at 10 % in project A's `production` | starting a rollout (POST /v1/tenants/01a0fb69-df6e-74e2-ab53-2ddee7731b6e/projects/01a0fb69-df72-7595-8c1a-70554b2e3530/environments/production/rollouts) does not exist on this server (HTTP 404: {"type":"urn:glossa:problem:not_found","title":"Not Found","status":404,"code":"not_found","detail":"no such resource"}) |
| · | the edge's manifest carries it | _not reached_ |
| · | three runtimes at 10 %, against the generator | _not reached_ |
| · | rollout support off | _not reached_ |
| · | advance to 50 % keeps every 10 % installation | _not reached_ |
| · | abort returns all of them to stable | _not reached_ |
| · | complete moves the pointer | _not reached_ |

The generator's table (`runtimes/testdata/rollout/cohorts.json`): 10000 installation ids; under its salt `c3RhZ2VkLXJvbGxvdXQtMQ`, 1004 are in the candidate at 10 %.

| | Phase | Runtime | In the candidate | Disagree with the generator | Note |
|---|---|---|---:|---:|---|
| ✅ | probe at 10 % | js | 1004 | 0 |  |
| ✅ | probe at 10 % | go | 1004 | 0 |  |
| ✅ | probe at 10 % | dart | 1004 | 0 |  |
| ✅ | probe, rollout support off | js | 0 | 0 |  |
| ✅ | probe, rollout support off | go | 0 | 0 |  |
| ✅ | probe, rollout support off | dart | 0 | 0 |  |

## §12.5 — audit export

| | Step | What happened |
|---|---|---|
| ❌ | the tenant's audit entries can be listed | listing audit entries (GET /v1/tenants/01a0fb69-df6e-74e2-ab53-2ddee7731b6e/audit-entries) does not exist on this server (HTTP 404: {"type":"urn:glossa:problem:not_found","title":"Not Found","status":404,"code":"not_found","detail":"no such resource"}) |
| ❌ | export the run's range as an audit export job | starting an audit export job (POST /v1/tenants/01a0fb69-df6e-74e2-ab53-2ddee7731b6e/audit-export-jobs) does not exist on this server (HTTP 404: {"type":"urn:glossa:problem:not_found","title":"Not Found","status":404,"code":"not_found","detail":"no such resource"}) |
| ❌ | `glossa audit verify` passes on the export, offline | there is no export to verify, and `glossa audit verify` exits 2: error: unknown command "audit" /   fix:   run `glossa help` for the list of commands |
| · | an entry for every call the harness recorded, with its actor | _not reached_ |
| · | one altered byte makes `glossa audit verify` fail | _not reached_ |
| · | no canary string appears anywhere in the export | _not reached_ |

The harness recorded 21 successful mutating calls in §12.1–§12.4 itself, never from the platform: map[12.1:10 12.2:1 12.3:6 12.4:4].

| § | Successful mutating calls the harness recorded |
|---|---:|
| 12.1 | 10 |
| 12.2 | 1 |
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
| ✅ | the three users are invitations with mapped roles and locales | held |
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

M2's exit test passed in 33s.

M3's exit test passed in 65s.

M4's exit test passed in 128s.

### M2 — **passed** in 33s

```text
--- PASS: TestM2Exit (30.88s)
--- PASS: TestFixtureIsCurrent (0.10s)
--- PASS: TestFixtureShape (0.00s)
--- PASS: TestInterchangeFilesRead (0.02s)
ok  	github.com/felixgeelhaar/glossa/platform/internal/systemtest/m2	31.273s
ok  	github.com/felixgeelhaar/glossa/platform/internal/systemtest/m2/fixture	0.373s
```

### M3 — **passed** in 65s

```text
--- PASS: TestM3Exit (61.74s)
--- PASS: TestFixtureIsCurrent (0.01s)
--- PASS: TestFixtureShape (0.00s)
ok  	github.com/felixgeelhaar/glossa/platform/internal/systemtest/m3	62.237s
ok  	github.com/felixgeelhaar/glossa/platform/internal/systemtest/m3/fixture	0.231s
```

### M4 — **passed** in 128s

```text
--- PASS: TestM4Exit (125.68s)
ok  	github.com/felixgeelhaar/glossa/platform/internal/systemtest/m4	126.071s
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
