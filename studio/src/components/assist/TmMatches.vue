<script setup lang="ts">
/**
 * Translation-memory matches for the message: score badge, how the
 * remembered source differs from this one (normalized: placeholders by
 * position), the target, where it came from, and insert (Mod+Alt+n).
 * Targets come in the editor's authoring syntax, written by the server's
 * kernel; one MF1 can't express comes as MF2, and says so.
 */
import { computed, onBeforeUnmount, ref, shallowRef, watch } from "vue";
import type { TMLookupResult, TMMatch } from "../../api/knowledge-schemas";
import { useKnowledge } from "../../api/knowledge";
import type { Message, ProjectLocale, Syntax } from "../../api/schemas";
import { ariaKeys, keyLabel } from "../../lib/shortcuts";
import { strings } from "../../strings";
import ErrorAlert from "../ErrorAlert.vue";
import SourceDiff from "../SourceDiff.vue";

const props = defineProps<{
  tenant: string;
  projectId: string;
  message: Message;
  source: ProjectLocale;
  target: ProjectLocale;
  /** The syntax the translation is being written in: targets come in it. */
  targetSyntax: Syntax;
  canInsert: boolean;
}>();
export interface MatchText {
  text: string;
  syntax: Syntax;
}
const emit = defineEmits<{ insert: [match: MatchText] }>();
const s = strings.tm;
const port = useKnowledge();

const result = shallowRef<TMLookupResult>();
const error = ref<unknown>(null);
const loading = ref(false);
let abort: AbortController | undefined;

async function load(): Promise<void> {
  abort?.abort();
  const a = (abort = new AbortController());
  loading.value = true;
  error.value = null;
  result.value = undefined;
  try {
    const r = await port.lookupTM(
      props.tenant,
      {
        source: props.message.source.text,
        syntax: props.message.source.syntax,
        source_locale: props.source.code,
        target_locale: props.target.code,
        project_id: props.projectId,
        message_key: props.message.key,
        namespace: props.message.namespace,
        limit: 5,
        min_score: 50,
        all_projects: false,
        // Showing a match isn't using it.
        count_hits: false,
        target_syntax: props.targetSyntax,
      },
      a.signal,
    );
    if (!a.signal.aborted) result.value = r;
  } catch (e) {
    if (!a.signal.aborted) error.value = e;
  } finally {
    if (!a.signal.aborted) loading.value = false;
  }
}
watch(() => [props.message.id, props.message.source_revision, props.target.code, props.targetSyntax], load, { immediate: true });
onBeforeUnmount(() => abort?.abort());

const matches = computed(() => result.value?.matches ?? []);
const tone = (m: TMMatch) => (m.score >= 100 ? "ok" : m.score >= 85 ? "accent" : "warn");
const chord = (i: number) => `${keyLabel("Mod")}${keyLabel("Alt")}${i + 1}`;

const textOf = (m: TMMatch): MatchText => ({ text: m.target_text, syntax: m.target_syntax });

/** The n-th match's target (1-based), for the Mod+Alt+n chord. */
function matchTarget(n: number): MatchText | undefined {
  const m = matches.value[n - 1];
  return m && textOf(m);
}
defineExpose({ matchTarget, reload: load });
</script>

<template>
  <section class="pane stack-sm" aria-labelledby="tm-h" data-testid="tm-pane">
    <h3 id="tm-h">{{ s.title }}</h3>
    <ErrorAlert :error="error" />
    <p v-if="loading" class="muted" role="status">{{ strings.app.loading }}</p>
    <p v-else-if="!error && !matches.length" class="muted">{{ s.none }}</p>
    <ol v-else class="matches">
      <li v-for="(m, i) in matches" :key="m.unit.id" class="match stack-sm" data-testid="tm-match">
        <div class="row">
          <span class="pill" :class="`pill-${tone(m)}`" data-testid="tm-score">{{ m.score }}</span>
          <span class="muted">{{ s.kind[m.kind] }}</span>
          <span class="spacer" />
          <button type="button" class="btn btn-sm" :disabled="!canInsert" :aria-keyshortcuts="i < 9 ? ariaKeys(['Mod', 'Alt', String(i + 1)]) : undefined" @click="emit('insert', textOf(m))">
            {{ s.insert }} <span v-if="i < 9" class="kbd-hint" aria-hidden="true">{{ chord(i) }}</span>
          </button>
        </div>
        <p class="target" :lang="target.code" :dir="target.direction">{{ m.target_text }}</p>
        <p v-if="m.target_syntax_fallback" class="hint" data-testid="tm-fallback">{{ s.fallbackMf2 }}</p>
        <div v-if="m.kind === 'fuzzy' && result" class="hint">
          <span>{{ s.sourceDiff }}</span>
          <SourceDiff :before="m.unit.source_normalized" :after="result.source_normalized" :lang="source.code" :dir="source.direction" />
        </div>
        <p class="hint">
          {{ m.unit.message_key ? s.from(m.unit.message_key) : s.imported }}
          · {{ m.unit.project_id ? s.scopeProject : s.scopeTenant }}
          <template v-if="!m.variables_adapted"> · {{ s.variablesKept }}</template>
        </p>
      </li>
    </ol>
  </section>
</template>

<style scoped>
.matches {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: var(--kl-space-3);
}
.match {
  padding: var(--kl-space-3);
  border: 1px solid var(--kl-border);
  border-radius: var(--kl-radius-md);
  background: var(--kl-surface);
}
.target {
  white-space: pre-wrap;
  font-family: var(--kl-font-mono);
  font-size: var(--kl-text-sm);
}
</style>
