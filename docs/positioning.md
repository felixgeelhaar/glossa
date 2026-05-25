# Glossa positioning brief

Captured 2026-05-25. Authoritative source for README hero copy, GHCR description, npm package descriptions, and the README of every consumer package. Reflect changes here in those surfaces within the same PR.

This brief follows April Dunford's five components: alternatives → capabilities → value → best-fit customer → category.

## Early Customer Profile (ECP)

**Solo founders + 2-5 person teams shipping a localized SaaS in the EU, comfortable self-hosting on k3s / single-node Linux, already using one or more of: Caddy / Plausible / Tolgee-adjacent stacks, and unwilling to pay per-translated-word for a tool that hosts strings their consumers will read.**

Not the ICP yet — Glossa has ~1 paying user (Felix). The ECP is the first 20 paying users; the ICP comes from observed patterns across those 20.

What the ECP has that Maya Voje's frame demands:
- **Willingness to pay** — they already pay for managed Postgres, managed object storage, or a Hetzner box. Translation management is a known cost line.
- **Burning pain** — they have copy in their app, they want it in a second language, they don't want to hand-edit `de.json` + `en.json` forever.
- **Proximity** — reachable via the EU indie-SaaS dev twitter / mastodon / discord scene + the open-source self-hosters community (Tolgee, Plausible, Caddy users).
- **Willingness to recommend** — this crowd shares tools openly.

What disqualifies a prospect:
- **Series A+ companies** — they'll buy Lokalise / Crowdin and pay per word without thinking. Glossa's wedge doesn't matter to them.
- **Non-self-hosters** — anyone who recoils at "you run the database" is a wrong fit for v0.1. (Managed Glossa is a 2027 option.)
- **English-only apps** — the AI fan-out + locale management story carries less weight when there's only one locale to manage.

## The 5 positioning components (Dunford)

### 1. Competitive alternatives

What the ECP would do without Glossa:

| Alternative | What they hate about it |
|---|---|
| **JSON files in git + PR review** (the default) | Translator can't edit without a PR; every locale change is a deploy; no live preview; no AI fan-out. |
| **Lokalise / Crowdin / Phrase** | Per-word billing scales with success in the wrong direction. Translator UX is fine; ops + billing are the friction. Cloud-only. |
| **Tolgee** (the close OSS competitor) | Closest match. Mature. But heavier ops footprint, no built-in AI fan-out with BYO LLM, no first-class web-components SDK. |
| **Build your own** | Six months of distraction from the product that pays the bills. |

### 2. Differentiated capabilities

Three capabilities Glossa has that the alternatives don't ship together:

1. **AI translator agents with BYO LLM key + per-row attribution.** OpenAI / Anthropic / Gemini / OpenAI-compatible configured per-tenant. Source-locale writes fan out automatically; every AI row is labeled `actor_kind="ai"` in the audit log. No per-word cost — you pay the LLM provider directly.
2. **Web components consumer SDK with SSE live updates.** `<glossa-text key="…">fallback</glossa-text>` drops into Vue, React, Svelte, Astro, plain HTML. Edits land in the browser within seconds without a redeploy. Fallback content stays visible offline.
3. **Self-hosted from day one with scoped read/write API keys.** Helm chart on OCI registry + raw k3s manifests + Docker Compose dev stack. Read-only keys safe to embed in static frontends; write keys live on the trusted server.

### 3. Differentiated value

What the capabilities mean for the ECP:

- **Predictable cost.** Two locales × 500 keys × 3 reviewers = $0 marginal cost on Glossa. Same workload on Lokalise: ~$120/mo.
- **No vendor lock-in.** Postgres dump out, JSON bundles out, every consumer fallback still renders if you turn Glossa off tomorrow.
- **Localized UX without re-deploys.** Marketing fixes the hero copy at 17:00 Friday; the running app reflects it by 17:00:05. Same flow translators use, no engineer involvement.
- **DSGVO posture handled.** EU-hosted, no third-party processor, self-managed encryption keys.

### 4. Best-fit target customer

Concretely:

- **Persona:** German-language indie SaaS founder, 1-3 person team, ships a B2C or low-touch B2B app in DE plus 1-3 other locales.
- **Where they live:** Hetzner / Netcup / OVH / their own basement. They know how to point an A record + write a values.yaml.
- **What they read:** indie hackers EU edition, mastodon, the Tolgee + Plausible + Caddy + Coolify discord communities.
- **What they've already adopted:** at least one of Plausible (analytics), Coolify (PaaS), Tolgee (i18n attempt that fell short).

### 5. Market category

**Self-hosted, AI-augmented translation management for indie EU SaaS.**

Tolgee defined "open-source translation management" — Glossa rides that category. The wedge is **AI fan-out with BYO LLM + web-components SDK + DSGVO-native posture**. Glossa is what Tolgee would be if it started in 2026 with cheap LLM access already on the table.

## Three sentences to land on

These three sentences should appear verbatim in the README hero, the GHCR image description, and the npm package descriptions:

1. **For small EU SaaS teams who want to localize without per-word fees,**
2. **Glossa is a self-hosted translation backend that ships AI fan-out, live SSE updates, and drop-in web components.**
3. **Unlike Lokalise or Crowdin which meter on word count, Glossa runs on your own k3s with your own LLM keys — no per-translation cost, no vendor lock-in.**

## Why these alternatives, not others

- **Why Lokalise as the reference paid alternative?** Most-recognized name in indie-SaaS founder mindshare. Crowdin / Phrase have similar shapes; one name keeps the comparison concrete.
- **Why Tolgee as the OSS reference?** Closest functional match. Other OSS i18n tools (Weblate, Pootle) target translator orgs, not product teams; the wedge is unclear there.
- **Why not "vs DIY JSON files"?** The DIY path is the real default; the comparison is in the value section. It's not the reference category — Glossa is a tool, not a competitor to git.

## Out of scope (Lochhead category-design discipline)

What Glossa is NOT, even if asked nicely:

- **Not a CAT tool.** No translation memory, no glossary management, no MT post-editing workflow optimization. Trados owns that category.
- **Not a content-management system.** Strings, not pages. No rich-text editor, no scheduled publishing.
- **Not enterprise.** No SSO, no SCIM, no SLA, no per-seat pricing. v1 stays self-host-only.

Surfacing these out-of-scope items in the README + docs prevents wrong-fit users from spending evaluation cycles on something we're never going to ship.

## Riskiest assumption (Doshi)

> "Self-hosters who want translation management already picked Tolgee, and the marginal pull of Glossa's AI fan-out / web-components / EU posture isn't enough to switch."

Validation plan:
1. **Run the second-user usability test** (task-usability-test) against someone currently using Tolgee. Specifically ask: would AI fan-out + the web-components SDK be enough to switch?
2. **Post a 'Glossa vs Tolgee, what would you pick?' thread** in the relevant communities and watch which dimensions resonate.
3. **If the answer is no:** category-creation gambit fails; pivot Glossa into a Tolgee plugin (ai-fan-out adapter + web-components SDK) rather than a competitor.
