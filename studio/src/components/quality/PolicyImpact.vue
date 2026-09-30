<script setup lang="ts">
/**
 * The impact preview (RFC 0005 §4.3): what a candidate policy would do
 * to the findings and the runs this project already has, answered by a
 * `dry_run` save that stores nothing.
 *
 * It exists for one failure mode: somebody adds `terminology: error`,
 * and forty open pull requests go red for something their authors
 * didn't do. So the number this panel leads with is not "findings
 * changed" but **open pull requests that would newly fail** — the
 * number of people who would wake up to a red pull request they did not
 * cause, and the one that decides whether the save ships with a grace.
 *
 * Every figure is a labelled number with its own sentence. Nothing here
 * is carried by colour alone: a zero says "Nothing changes", and the
 * per-rule table names a rule that changed nothing rather than omitting
 * it.
 */
import { computed } from "vue";
import type { PolicyImpact, PolicyRule } from "../../api/policy-schemas";
import { namedFields } from "../../lib/policy";
import { strings } from "../../strings";

const props = defineProps<{ impact: PolicyImpact; stale: boolean }>();
const s = strings.policy;
const c = s.impactColumns;

/** The counts in the order a reader needs them: the ones that break people first. */
const numbers = computed(() => [
  { key: "open-pull-requests", label: s.impactOpenPullRequests, value: props.impact.open_pull_requests, alarming: true },
  { key: "newly-failing", label: s.impactNewlyFailing, value: props.impact.newly_failing, alarming: true },
  { key: "raised", label: s.impactRaised, value: props.impact.raised, alarming: false },
  { key: "silenced", label: s.impactSilenced, value: props.impact.silenced, alarming: false },
  { key: "lowered", label: s.impactLowered, value: props.impact.lowered, alarming: false },
  { key: "no-longer-failing", label: s.impactNoLongerFailing, value: props.impact.no_longer_failing, alarming: false },
]);

const selector = (r: PolicyRule | undefined): string => {
  if (!r) return s.ruleSelectsEverything;
  const fields = namedFields(r);
  return fields.length ? fields.map(([k, v]) => `${s.ruleField[k] ?? k}: ${v}`).join(" · ") : s.ruleSelectsEverything;
};
</script>

<template>
  <section class="card stack-sm" aria-labelledby="impact-h" data-testid="policy-impact">
    <h2 id="impact-h">{{ s.previewTitle }}</h2>
    <p class="muted">{{ s.previewLead }}</p>
    <p v-if="stale" class="alert alert-warn" role="status" data-testid="impact-stale">{{ s.previewStale }}</p>

    <p v-if="impact.findings === 0" class="hint" data-testid="impact-nothing-measured">{{ s.previewNothingToMeasure }}</p>
    <p v-else class="hint" data-testid="impact-measured">{{ s.previewMeasured(impact.findings, impact.runs) }}</p>

    <dl class="numbers">
      <div v-for="x in numbers" :key="x.key" class="number" :data-testid="`impact-${x.key}`">
        <dt>{{ x.label }}</dt>
        <dd>
          <span class="pill" :class="x.value === 0 ? 'pill-neutral' : x.alarming ? 'pill-err' : 'pill-accent'">
            <!-- The word is the answer; the pill only repeats it. -->
            {{ x.value === 0 ? s.impactNone : x.value.toLocaleString() }}
          </span>
        </dd>
      </div>
    </dl>

    <p v-if="impact.open_pull_requests > 0" class="hint" data-testid="impact-open-prs-hint">{{ s.impactOpenPullRequestsHint }}</p>
    <p v-if="impact.newly_failing_refs?.length" class="refs" data-testid="impact-newly-failing-refs">{{ s.impactRefs(impact.newly_failing_refs.join(", ")) }}</p>
    <p v-if="impact.no_longer_failing_refs?.length" class="refs" data-testid="impact-fixed-refs">
      {{ s.impactRefsFixed(impact.no_longer_failing_refs.join(", ")) }}
    </p>

    <template v-if="impact.rules.length">
      <h3>{{ s.impactRulesTitle }}</h3>
      <p class="muted">{{ s.impactRulesLead }}</p>
      <div class="scroll">
        <table class="table" data-testid="impact-rules">
          <thead>
            <tr>
              <th scope="col">{{ c.rule }}</th>
              <th scope="col">{{ c.selector }}</th>
              <th scope="col">{{ c.matched }}</th>
              <th scope="col">{{ c.changed }}</th>
              <th scope="col">{{ c.failing }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="r in impact.rules" :key="r.rule" :data-rule="r.rule" data-testid="impact-rule">
              <th scope="row">{{ r.rule + 1 }}</th>
              <td class="selector">{{ selector(r.selector) }}</td>
              <td class="num">{{ r.matched.toLocaleString() }}</td>
              <td class="num">
                {{ r.changed.toLocaleString() }}
                <span v-if="r.changed === 0 && r.newly_failing === 0" class="visually-hidden">{{ s.impactRuleUnchanged }}</span>
              </td>
              <td class="num">{{ r.newly_failing.toLocaleString() }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </template>
  </section>
</template>

<style scoped>
.numbers {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(13rem, 1fr));
  gap: var(--kl-space-3);
  margin: 0;
}
.number {
  display: flex;
  flex-direction: column;
  gap: var(--kl-space-1);
}
dt {
  font-size: var(--kl-text-sm);
  color: var(--kl-ink-secondary);
}
dd {
  margin: 0;
}
.refs {
  overflow-wrap: anywhere;
}
.scroll {
  overflow-x: auto;
}
.selector {
  overflow-wrap: anywhere;
  max-inline-size: 24rem;
}
.num {
  text-align: end;
  font-variant-numeric: tabular-nums;
}
</style>
