<script setup lang="ts">
/**
 * One import and its results (RFC 0003 §6): the summary by status and
 * kind, then every item in file order — filterable by status and kind,
 * with where in the file (line, column) and why a conflict or invalid
 * item is one. A dry run can be applied for real: the same file (kept
 * in memory from the wizard, else chosen again and checked by SHA-256)
 * with the same options as a merge. An import that reused an earlier
 * one's result says so and links it.
 */
import { computed, onBeforeUnmount, ref, shallowRef, watch } from "vue";
import { RouterLink, useRoute, useRouter } from "vue-router";
import { useIntegration } from "../../api/integration";
import type { ImportJob, ImportResult, ImportResultKind, ImportResultStatus } from "../../api/integration-schemas";
import ErrorAlert from "../../components/ErrorAlert.vue";
import { failureText, resultTone, resultWhy, stateTone } from "../../components/integration/jobs";
import JobProgress from "../../components/integration/JobProgress.vue";
import { useImportRun } from "../../components/integration/useImportRun";
import { bytes, canCancelImport, canImportAny, isTerminal, polling, pollUntil, rememberedFile, sha256Hex } from "../../lib/integration";
import { absoluteTime, relativeTime } from "../../lib/time";
import { usePeople } from "../../session/people";
import { strings } from "../../strings";
import { useProject } from "./context";

const route = useRoute();
const router = useRouter();
const { tenant, projectId, grant } = useProject();
const port = useIntegration();
const people = usePeople(() => tenant.value);
const s = strings.integration;
const r = s.results;
const STATUSES: ImportResultStatus[] = ["created", "updated", "unchanged", "conflict", "invalid"];
const KINDS: ImportResultKind[] = ["message", "translation", "tm_unit", "concept"];

const jobId = computed(() => String(route.params.job));
const job = shallowRef<ImportJob>();
const error = ref<unknown>(null);
const status = ref<ImportResultStatus | "">("");
const kind = ref<ImportResultKind | "">("");
const items = shallowRef<ImportResult[]>([]);
const next = ref<string>();
const loadingItems = ref(false);
let poller: AbortController | undefined;

const name = computed(() => job.value?.file_name || s.unnamed);
const kindsPresent = computed(() => Object.keys(job.value?.summary.by_kind ?? {}) as ImportResultKind[]);

async function loadItems(more = false): Promise<void> {
  const j = job.value;
  if (!j) return;
  loadingItems.value = true;
  try {
    const q = { ...(status.value ? { status: status.value } : {}), ...(kind.value ? { kind: kind.value } : {}) };
    const p = await port.importResults(tenant.value, j.id, q, more ? next.value : undefined);
    items.value = more ? [...items.value, ...p.items] : p.items;
    next.value = p.next;
  } catch (e) {
    error.value = e;
  } finally {
    loadingItems.value = false;
  }
}

async function load(): Promise<void> {
  poller?.abort();
  const ctrl = (poller = new AbortController());
  error.value = null;
  job.value = undefined;
  items.value = [];
  status.value = "";
  kind.value = "";
  try {
    const first = await port.importJob(tenant.value, jobId.value);
    job.value = first;
    // A job still waiting for its file (abandoned uploads) isn't polled: nothing moves it but its requester.
    if (first.state === "queued" || first.state === "running") {
      await pollUntil(() => port.importJob(tenant.value, jobId.value), (j) => isTerminal(j.state), (j) => (job.value = j), polling.ms, ctrl.signal);
    }
    await loadItems();
  } catch (e) {
    if (!(e instanceof DOMException && e.name === "AbortError")) error.value = e;
  }
}
watch(jobId, load, { immediate: true });
watch([status, kind], () => void loadItems());
onBeforeUnmount(() => poller?.abort());

async function cancel(): Promise<void> {
  const j = job.value;
  if (!j) return;
  try {
    job.value = await port.cancelImport(tenant.value, j.id);
  } catch (e) {
    error.value = e;
  }
}

// ── apply a dry run ───────────────────────────────────────────────────
const runner = useImportRun(port, tenant);
const canApply = computed(() => job.value?.mode === "dry_run" && job.value.state === "succeeded" && canImportAny(grant.value));
const remembered = computed(() => (job.value ? rememberedFile(job.value.id) : undefined));
const pickError = ref("");

async function apply(file: File | undefined): Promise<void> {
  const j = job.value;
  if (!j || !file) return;
  pickError.value = "";
  if (j.file && (await sha256Hex(file)) !== j.file.sha256) {
    pickError.value = r.applyWrongFile;
    return;
  }
  const done = await runner.run(
    {
      format: j.format,
      mode: "merge",
      file_name: j.file_name,
      ...(j.project_id ? { project_id: j.project_id } : {}),
      ...(Object.keys(j.options).length ? { options: j.options } : {}),
    },
    file,
  );
  if (done) await router.push({ name: "import-job", params: { tenant: tenant.value, project: projectId.value, job: done.id } });
}
const onPick = (e: Event) => void apply((e.target as HTMLInputElement).files?.[0]);
</script>

<template>
  <div class="page stack">
    <div class="stack-sm">
      <RouterLink :to="{ name: 'files', params: { tenant, project: projectId } }">← {{ s.wizard.back }}</RouterLink>
      <h1>{{ r.title(name) }}</h1>
      <p v-if="job" class="row hint">
        <span class="pill pill-neutral">{{ s.format[job.format] }}</span>
        <span class="pill" :class="job.mode === 'overwrite' ? 'pill-warn' : 'pill-neutral'">{{ s.mode[job.mode] }}</span>
        <span class="pill" :class="stateTone(job.state)" data-testid="import-state">{{ s.state[job.state] }}</span>
        <span :title="absoluteTime(job.created_at)">{{ r.requested(people(job.created_by), relativeTime(job.created_at)) }}</span>
        <span v-if="job.file" class="mono">{{ r.file(bytes(job.file.size), job.file.sha256) }}</span>
      </p>
    </div>
    <ErrorAlert :error="error" />

    <template v-if="job">
      <JobProgress
        v-if="!isTerminal(job.state)"
        :text="job.cancel_requested ? s.cancelRequested : s.wizard.running(job.processed_items, job.total_items)"
        :value="job.total_items ? job.processed_items / job.total_items : undefined"
        :label="s.wizard.progress"
        :cancellable="canCancelImport(job)"
        @cancel="cancel"
      />
      <p v-if="job.state === 'failed'" class="alert alert-error" role="alert">{{ r.failed(failureText(job.failure_code, job.failure_message)) }}</p>
      <p v-if="job.state === 'cancelled'" class="alert">{{ r.cancelled }}</p>
      <div v-if="job.reused_job_id" class="alert alert-warn stack-sm" data-testid="reused-notice">
        <p>{{ r.reused }}</p>
        <RouterLink :to="{ name: 'import-job', params: { tenant, project: projectId, job: job.reused_job_id } }">{{ r.reusedLink }}</RouterLink>
      </div>

      <section v-if="job.mode === 'dry_run' && job.state === 'succeeded'" class="card stack-sm dry-run" data-testid="dry-run-notice">
        <p class="alert-title">{{ r.dryRun }}</p>
        <template v-if="canApply">
          <p class="hint">{{ r.applyHint }}</p>
          <JobProgress v-if="runner.phase.value !== 'idle'" :text="runner.progressText.value" :value="runner.progressValue.value" :label="s.wizard.progress" cancellable @cancel="runner.cancel()" />
          <div v-else-if="remembered" class="row">
            <button type="button" class="btn btn-primary" @click="apply(remembered)">{{ r.apply }}</button>
          </div>
          <div v-else class="field">
            <label for="apply-file">{{ r.applyChoose }}</label>
            <input id="apply-file" type="file" @change="onPick" />
          </div>
          <p v-if="pickError" class="field-error" role="alert">{{ pickError }}</p>
          <ErrorAlert :error="runner.error.value" />
        </template>
      </section>

      <section v-if="isTerminal(job.state)" class="stack" :aria-label="r.summary">
        <h2>{{ r.summary }}</h2>
        <dl class="tiles" data-testid="import-summary">
          <div v-for="st in STATUSES" :key="st" class="tile" :data-status="st">
            <dt>{{ r.status[st] }}</dt>
            <dd>{{ job.summary[st] }}</dd>
          </div>
        </dl>
        <div v-if="kindsPresent.length > 1" class="scroll">
          <table class="table">
            <caption class="visually-hidden">{{ r.summary }}</caption>
            <thead>
              <tr>
                <th scope="col">{{ r.kindColumn }}</th>
                <th v-for="st in STATUSES" :key="st" scope="col">{{ r.status[st] }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="k in kindsPresent" :key="k">
                <th scope="row">{{ r.kind[k] ?? k }}</th>
                <td v-for="st in STATUSES" :key="st">{{ job.summary.by_kind[k]?.[st] ?? 0 }}</td>
              </tr>
            </tbody>
          </table>
        </div>

        <h2>{{ r.items }}</h2>
        <div class="row filters">
          <div class="field">
            <label for="res-status">{{ r.filterStatus }}</label>
            <select id="res-status" v-model="status">
              <option value="">{{ r.all }}</option>
              <option v-for="st in STATUSES" :key="st" :value="st">{{ r.status[st] }} ({{ job.summary[st] }})</option>
            </select>
          </div>
          <div class="field">
            <label for="res-kind">{{ r.filterKind }}</label>
            <select id="res-kind" v-model="kind">
              <option value="">{{ r.all }}</option>
              <option v-for="k in KINDS" :key="k" :value="k">{{ r.kind[k] }}</option>
            </select>
          </div>
        </div>
        <p v-if="!items.length && !loadingItems" class="card muted">{{ r.none }}</p>
        <div v-else class="scroll">
          <table class="table" data-testid="import-results">
            <caption class="visually-hidden">{{ r.items }}</caption>
            <thead>
              <tr>
                <th scope="col">{{ r.filterStatus }}</th>
                <th scope="col">{{ r.filterKind }}</th>
                <th scope="col">{{ r.key }}</th>
                <th scope="col">{{ r.locale }}</th>
                <th scope="col">{{ r.where }}</th>
                <th scope="col">{{ r.why }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="it in items" :key="it.seq" :data-status="it.status">
                <td><span class="pill" :class="resultTone[it.status]">{{ r.status[it.status] }}</span></td>
                <td>{{ r.kind[it.kind] ?? it.kind }}</td>
                <td class="mono key">{{ it.key }}</td>
                <td class="mono">{{ it.locale ?? "—" }}</td>
                <td>{{ it.line ? r.at(it.line, it.column) : "—" }}</td>
                <td>{{ resultWhy(it.code, it.detail) || "—" }}</td>
              </tr>
            </tbody>
          </table>
        </div>
        <button v-if="next" type="button" class="btn load-more" :disabled="loadingItems" @click="loadItems(true)">{{ s.loadMore }}</button>
      </section>
    </template>
    <p v-else-if="!error" class="muted" role="status">{{ strings.app.loading }}</p>
  </div>
</template>

<style scoped>
.tiles {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(8rem, 1fr));
  gap: var(--kl-space-3);
  margin: 0;
}
.tile {
  border: 1px solid var(--kl-border);
  border-radius: var(--kl-radius-md);
  padding: var(--kl-space-3) var(--kl-space-4);
  background: var(--kl-surface-raised);
}
.tile dt {
  font-size: var(--kl-text-sm);
  color: var(--kl-ink-secondary);
}
.tile dd {
  margin: 0;
  font-size: var(--kl-text-lg);
  font-weight: var(--kl-weight-semibold);
}
.tile[data-status="conflict"] {
  border-color: var(--gs-warn);
}
.tile[data-status="invalid"] {
  border-color: var(--gs-err);
}
.dry-run {
  border-color: var(--kl-accent-border);
}
.filters {
  align-items: flex-end;
}
.scroll {
  overflow-x: auto;
}
.key {
  word-break: break-all;
}
.load-more {
  align-self: flex-start;
}
</style>
