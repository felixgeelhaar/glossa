<script setup lang="ts">
import { computed, reactive, ref, watch } from "vue";
import type { ProjectRef, ReleasesPort } from "../../api/releases";
import { isApiError } from "../../api/errors";
import type { ShippableState } from "../../api/schemas";
import { SHIPPABLE_STATES } from "../../lib/releases";
import { strings } from "../../strings";
import ErrorAlert from "../ErrorAlert.vue";
import ModalDialog from "../ModalDialog.vue";

const props = defineProps<{ open: boolean; port: ReleasesPort; project: ProjectRef; environment: string }>();
const emit = defineEmits<{ close: []; done: [message: string] }>();
const s = strings.releases;

const form = reactive({ states: [] as ShippableState[], include_outdated: true });
const etag = ref<string>();
const loading = ref(false);
const busy = ref(false);
const error = ref<unknown>(null);
const valid = computed(() => form.states.length > 0);

async function load(): Promise<void> {
  loading.value = true;
  try {
    const r = await props.port.environment(props.project, props.environment);
    form.states = [...r.value.policy.states];
    form.include_outdated = r.value.policy.include_outdated;
    etag.value = r.etag;
  } catch (e) {
    error.value = e;
  } finally {
    loading.value = false;
  }
}

watch(
  () => props.open,
  (open) => {
    if (!open) return;
    error.value = null;
    void load();
  },
  { immediate: true },
);

async function save(): Promise<void> {
  if (!valid.value || !etag.value) return;
  busy.value = true;
  error.value = null;
  try {
    const states = SHIPPABLE_STATES.filter((st) => form.states.includes(st));
    await props.port.updatePolicy(props.project, props.environment, { states, include_outdated: form.include_outdated }, etag.value);
    emit("done", s.policySaved(props.environment));
  } catch (e) {
    error.value = e;
    if (isApiError(e, "precondition_failed")) await load();
  } finally {
    busy.value = false;
  }
}
</script>

<template>
  <ModalDialog :open="open" :title="s.policyTitle(environment)" @close="emit('close')">
    <p v-if="loading" class="muted" role="status">{{ strings.app.loading }}</p>
    <form v-else id="policy-form" class="stack" @submit.prevent="save">
      <fieldset class="stack-sm" aria-describedby="pol-states-hint">
        <legend class="label">{{ s.policyStates }}</legend>
        <label v-for="st in SHIPPABLE_STATES" :key="st" class="check">
          <input v-model="form.states" type="checkbox" :value="st" :disabled="busy" />
          <span>{{ s.state[st] }}</span>
        </label>
        <span id="pol-states-hint" class="hint">{{ s.policyStatesHint }}</span>
        <span v-if="!valid" class="field-error" role="alert">{{ s.policyNeedsState }}</span>
      </fieldset>
      <label class="check">
        <input v-model="form.include_outdated" type="checkbox" :disabled="busy" aria-describedby="pol-outdated-hint" />
        <span class="stack-sm">
          <span>{{ s.includeOutdated }}</span>
          <span id="pol-outdated-hint" class="hint">{{ s.includeOutdatedHint }}</span>
        </span>
      </label>
    </form>
    <ErrorAlert :error="error" />
    <template #actions>
      <button type="button" class="btn" :disabled="busy" @click="emit('close')">{{ strings.app.cancel }}</button>
      <button type="submit" form="policy-form" class="btn btn-primary" :disabled="busy || loading || !valid">{{ s.savePolicy }}</button>
    </template>
  </ModalDialog>
</template>

<style scoped>
fieldset {
  border: none;
  margin: 0;
  padding: 0;
}
</style>
