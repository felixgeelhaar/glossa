<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { newIdempotencyKey, type ProjectRef, type ReleasesPort } from "../../api/releases";
import type { Release, ReleaseDiff } from "../../api/schemas";
import { isEmptyDiff } from "../../lib/releases";
import { strings } from "../../strings";
import ErrorAlert from "../ErrorAlert.vue";
import ModalDialog from "../ModalDialog.vue";
import { policyText, type ReleaseBook } from "./book";
import LocaleTable from "./LocaleTable.vue";

const props = defineProps<{ open: boolean; port: ReleasesPort; project: ProjectRef; book: ReleaseBook; environment?: string | undefined }>();
const emit = defineEmits<{ close: []; published: [release: Release] }>();
const s = strings.releases;

const env = ref("development");
const note = ref("");
const busy = ref(false);
const error = ref<unknown>(null);
const result = ref<Release>();
const resultDiff = ref<ReleaseDiff>();
let key = newIdempotencyKey();

watch(
  () => props.open,
  (open) => {
    if (!open) return;
    env.value = props.environment ?? props.book.environments.value[0]?.name ?? "development";
    note.value = "";
    error.value = null;
    result.value = undefined;
    resultDiff.value = undefined;
    key = newIdempotencyKey();
  },
  { immediate: true },
);
// A different request needs a different key; a retry of the same one reuses it.
watch([env, note], () => {
  key = newIdempotencyKey();
});

const target = computed(() => props.book.env(env.value));
const current = computed(() => props.book.byId.value.get(target.value?.current_release_id ?? ""));
const localeCodes = (r: Release) => r.locales.map((l) => l.code);

async function publish(): Promise<void> {
  busy.value = true;
  error.value = null;
  try {
    const r = await props.port.publish(props.project, { environment: env.value, note: note.value.trim() || undefined }, key);
    result.value = r;
    emit("published", r);
    if (r.parent_id) {
      [resultDiff.value] = await Promise.all([props.port.diff(props.project, r.id), props.book.ensure([r.parent_id])]);
    }
  } catch (e) {
    error.value = e;
  } finally {
    busy.value = false;
  }
}
</script>

<template>
  <ModalDialog :open="open" :title="s.publishTitle" wide @close="emit('close')">
    <template v-if="!result">
      <form id="publish-form" class="stack" @submit.prevent="publish">
        <div class="field">
          <label for="pub-env">{{ s.environment }}</label>
          <select id="pub-env" v-model="env" :disabled="busy">
            <option v-for="e in book.environments.value" :key="e.name" :value="e.name">{{ e.name }}</option>
          </select>
        </div>
        <div class="field">
          <label for="pub-note">{{ s.note }}</label>
          <textarea id="pub-note" v-model="note" rows="2" maxlength="1000" aria-describedby="pub-note-hint" :disabled="busy" />
          <span id="pub-note-hint" class="hint">{{ s.noteHint }}</span>
        </div>
      </form>
      <section class="stack-sm" aria-labelledby="pub-ships-h" data-testid="publish-preview">
        <h3 id="pub-ships-h">{{ s.willShip }}</h3>
        <p v-if="target">{{ s.policyLead(env, policyText(target.policy)) }}</p>
        <template v-if="current">
          <p>{{ s.currentlyServing(env, s.version(current.version)) }}</p>
          <LocaleTable
            :locales="localeCodes(current)"
            :source-locale="current.source_locale"
            :counts="current.counts.locales"
            :caption="s.currentlyServing(env, s.version(current.version))"
          />
        </template>
        <p v-else>{{ s.firstRelease(env) }}</p>
        <p class="muted">{{ s.exactAfter }}</p>
      </section>
    </template>
    <template v-else>
      <p class="alert alert-ok" role="status">{{ s.published(s.version(result.version), result.environment) }}</p>
      <template v-if="resultDiff">
        <h3>{{ s.changesSince(book.label(result.parent_id)) }}</h3>
        <p v-if="isEmptyDiff(resultDiff)" class="muted">{{ s.noChanges }}</p>
        <LocaleTable
          :locales="localeCodes(result)"
          :source-locale="result.source_locale"
          :counts="result.counts.locales"
          :diff="resultDiff"
          :caption="s.changesSince(book.label(result.parent_id))"
        />
      </template>
      <template v-else>
        <p class="muted">{{ s.everythingNew }}</p>
        <LocaleTable
          :locales="localeCodes(result)"
          :source-locale="result.source_locale"
          :counts="result.counts.locales"
          :caption="s.published(s.version(result.version), result.environment)"
        />
      </template>
    </template>
    <ErrorAlert :error="error" />
    <template #actions>
      <template v-if="!result">
        <button type="button" class="btn" :disabled="busy" @click="emit('close')">{{ strings.app.cancel }}</button>
        <button type="submit" form="publish-form" class="btn btn-primary" :disabled="busy">{{ busy ? s.publishing : s.publishTo(env) }}</button>
      </template>
      <button v-else type="button" class="btn btn-primary" @click="emit('close')">{{ s.done }}</button>
    </template>
  </ModalDialog>
</template>
