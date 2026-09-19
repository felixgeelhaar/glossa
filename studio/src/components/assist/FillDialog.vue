<script setup lang="ts">
/**
 * "Fill with AI" for the workspace's current filter: queues one job per
 * missing (or outdated) message in the locale, then follows the fill's
 * jobs (`ai-jobs?fill=`) until they settle. The server's warnings say up
 * front when jobs will do little (consent off, no budget, no provider).
 */
import { computed, onBeforeUnmount, ref, shallowRef, watch } from "vue";
import { RouterLink } from "vue-router";
import type { AIFill, AIJob } from "../../api/intelligence-schemas";
import { useIntelligence } from "../../api/intelligence";
import { newIdempotencyKey } from "../../api/releases";
import { fillProgress, FILL_POLL_MS } from "../../lib/fill";
import { strings } from "../../strings";
import ErrorAlert from "../ErrorAlert.vue";
import ModalDialog from "../ModalDialog.vue";

const props = defineProps<{
  open: boolean;
  tenant: string;
  projectId: string;
  locale: string;
  namespace: string | undefined;
  outdated: boolean;
  /** The keys the search narrows the list to, when it does. */
  keys: string[] | undefined;
}>();
const emit = defineEmits<{ close: []; settled: [fill: AIFill] }>();
const s = strings.fill;
const port = useIntelligence();

const includeOutdated = ref(props.outdated);
const fill = shallowRef<AIFill>();
const jobs = shallowRef<AIJob[]>([]);
const error = ref<unknown>(null);
const busy = ref(false);
let key = newIdempotencyKey();
let timer: ReturnType<typeof setTimeout> | undefined;

watch(
  () => props.open,
  (open) => {
    if (!open) return stop();
    fill.value = undefined;
    jobs.value = [];
    error.value = null;
    includeOutdated.value = props.outdated;
    key = newIdempotencyKey();
  },
);

const progress = computed(() => fillProgress(jobs.value));

function stop(): void {
  clearTimeout(timer);
  timer = undefined;
}

async function poll(): Promise<void> {
  const f = fill.value;
  if (!f) return;
  try {
    jobs.value = await port.jobs(props.tenant, { fill: f.id });
    error.value = null;
  } catch (e) {
    error.value = e;
  }
  if (progress.value.active > 0 && props.open) timer = setTimeout(() => void poll(), FILL_POLL_MS);
  else if (progress.value.active === 0) emit("settled", f);
}

async function start(): Promise<void> {
  busy.value = true;
  error.value = null;
  try {
    fill.value = await port.createFill(
      { tenant: props.tenant, project: props.projectId },
      {
        locales: [props.locale],
        ...(props.namespace ? { namespace: props.namespace } : {}),
        ...(props.keys ? { keys: props.keys } : {}),
        include_outdated: includeOutdated.value,
      },
      key,
    );
    await poll();
  } catch (e) {
    error.value = e;
  } finally {
    busy.value = false;
  }
}

async function cancel(): Promise<void> {
  const f = fill.value;
  if (!f) return;
  busy.value = true;
  try {
    fill.value = await port.cancelFill(props.tenant, f.id);
    stop();
    await poll();
  } catch (e) {
    error.value = e;
  } finally {
    busy.value = false;
  }
}
onBeforeUnmount(stop);

const skipped = computed(() => Object.entries(fill.value?.skipped ?? {}).filter(([, n]) => n > 0));
</script>

<template>
  <ModalDialog :open="open" :title="s.title(locale)" @close="emit('close')">
    <template v-if="!fill">
      <p>{{ s.scope(locale, namespace, keys?.length) }}</p>
      <label class="check">
        <input v-model="includeOutdated" type="checkbox" />
        <span>{{ s.includeOutdated }}</span>
      </label>
      <p class="hint">{{ s.lead }}</p>
    </template>
    <div v-else class="stack-sm" data-testid="fill-progress">
      <ul v-if="fill.warnings.length" class="stack-sm warnings" data-testid="fill-warnings">
        <li v-for="w in fill.warnings" :key="w" class="alert alert-warn">{{ s.warning(w) }}</li>
      </ul>
      <p>{{ s.queued(fill.jobs_created, fill.jobs_existing) }}</p>
      <p v-if="skipped.length" class="hint">{{ s.skipped(skipped) }}</p>
      <template v-if="progress.total">
        <label for="fill-bar" class="label">{{ s.progressLabel }}</label>
        <progress id="fill-bar" :max="progress.total" :value="progress.total - progress.active" />
        <p role="status" data-testid="fill-status">{{ s.progress(progress) }}</p>
        <ul v-if="progress.failures.length" class="hint failures">
          <li v-for="[code, n] in progress.failures" :key="code">{{ s.failure(code, n) }}</li>
        </ul>
      </template>
      <p v-else role="status" data-testid="fill-status">{{ s.nothing }}</p>
    </div>
    <ErrorAlert :error="error" />
    <template #actions>
      <template v-if="!fill">
        <button type="button" class="btn" @click="emit('close')">{{ strings.app.cancel }}</button>
        <button type="button" class="btn btn-primary" :disabled="busy" @click="start">{{ s.start }}</button>
      </template>
      <template v-else>
        <button v-if="progress.queued > 0" type="button" class="btn btn-danger" :disabled="busy" @click="cancel">{{ s.cancelQueued }}</button>
        <RouterLink v-if="progress.active === 0 && progress.succeeded > 0" class="btn" :to="{ name: 'review', params: { tenant, project: projectId }, query: { locale } }">{{ s.openQueue }}</RouterLink>
        <button type="button" class="btn btn-primary" @click="emit('close')">{{ progress.active ? s.keepRunning : strings.app.close }}</button>
      </template>
    </template>
  </ModalDialog>
</template>

<style scoped>
.warnings {
  list-style: none;
  margin: 0;
  padding: 0;
}
progress {
  inline-size: 100%;
  accent-color: var(--kl-accent);
}
.failures {
  margin: 0;
  padding-inline-start: var(--kl-space-5);
}
</style>
