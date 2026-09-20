<script setup lang="ts">
/**
 * The AI suggestion for this message and locale: the newest one, with
 * accept (as is), edit and accept (sends the edited MF2 `text`, so the
 * structured diff feeds the metrics) and reject.
 */
import { computed, nextTick, ref, shallowRef, useTemplateRef, watch } from "vue";
import { isApiError } from "../../api/errors";
import type { AISuggestion } from "../../api/intelligence-schemas";
import { useIntelligence } from "../../api/intelligence";
import type { Message, ProjectLocale } from "../../api/schemas";
import { ariaKeys, keyLabel } from "../../lib/shortcuts";
import { allowsFor, type Grant } from "../../session/permissions";
import { problemText, strings } from "../../strings";
import ErrorAlert from "../ErrorAlert.vue";
import SuggestionCard from "./SuggestionCard.vue";

const props = defineProps<{ tenant: string; projectId: string; message: Message; target: ProjectLocale; grant: Grant }>();
const emit = defineEmits<{ accepted: [suggestion: AISuggestion] }>();
const s = strings.ai;
const port = useIntelligence();

const suggestion = shallowRef<AISuggestion>();
const loading = ref(false);
const error = ref<unknown>(null);
const notice = ref("");
const busy = ref(false);
const editing = ref(false);
const editText = ref("");
const rejecting = ref(false);
const reason = ref("");
const editArea = useTemplateRef<HTMLTextAreaElement>("editArea");

const canDecide = computed(
  () => allowsFor(props.grant, "intelligence.translate", props.target.code) && allowsFor(props.grant, "translations.write", props.target.code),
);
const pending = computed(() => suggestion.value?.status === "pending");

let seq = 0;
async function load(): Promise<void> {
  const n = ++seq;
  loading.value = true;
  error.value = null;
  try {
    const page = await port.suggestions(props.tenant, { project: props.projectId, message: props.message.id, locale: props.target.code }, 1);
    if (n === seq) suggestion.value = page.items[0];
  } catch (e) {
    if (n === seq) error.value = e;
  } finally {
    if (n === seq) loading.value = false;
  }
}
watch(
  () => [props.message.id, props.target.code],
  () => {
    suggestion.value = undefined;
    editing.value = false;
    rejecting.value = false;
    notice.value = "";
    void load();
  },
  { immediate: true },
);

async function decide(run: (id: string) => Promise<AISuggestion>, done: string): Promise<void> {
  const current = suggestion.value;
  if (!current || busy.value || !canDecide.value) return;
  busy.value = true;
  error.value = null;
  notice.value = "";
  try {
    const r = await run(current.id);
    suggestion.value = r;
    editing.value = false;
    rejecting.value = false;
    notice.value = done;
    if (r.status === "accepted") emit("accepted", r);
  } catch (e) {
    if (isApiError(e, "suggestion_decided") || isApiError(e, "suggestion_outdated")) {
      await load();
      error.value = new Error(problemText(e));
    } else error.value = e;
  } finally {
    busy.value = false;
  }
}

const accept = () => decide((id) => port.accept(props.tenant, id), s.acceptedNotice);
const acceptEdit = () => decide((id) => port.accept(props.tenant, id, { text: editText.value, syntax: "mf2" }), s.acceptedEditNotice);
const reject = () => decide((id) => port.reject(props.tenant, id, reason.value.trim() || undefined), s.rejectedNotice);

async function startEdit(): Promise<void> {
  if (!pending.value || !canDecide.value) return;
  editText.value = suggestion.value?.message ?? "";
  editing.value = true;
  rejecting.value = false;
  await nextTick();
  editArea.value?.focus();
}

function onEditKey(e: KeyboardEvent): void {
  if (e.key === "Enter" && (e.metaKey || e.ctrlKey) && !e.altKey) {
    e.preventDefault();
    void acceptEdit();
  } else if (e.key === "Escape") {
    e.preventDefault();
    editing.value = false;
  }
}

defineExpose({
  reload: load,
  /** Mod+Alt+Enter from the editor. */
  accept: () => (pending.value ? accept() : undefined),
  /** Mod+Alt+0 from the editor. */
  edit: startEdit,
  hasPending: () => pending.value && canDecide.value,
});
</script>

<template>
  <section class="pane stack-sm" aria-labelledby="ai-h" data-testid="ai-panel">
    <div class="row">
      <h3 id="ai-h">{{ s.title }}</h3>
      <span v-if="suggestion" class="pill" :class="pending ? 'pill-accent' : 'pill-neutral'" data-testid="suggestion-status">{{ s.status[suggestion.status] }}</span>
    </div>
    <ErrorAlert :error="error" />
    <p v-if="loading && !suggestion" class="muted" role="status">{{ strings.app.loading }}</p>
    <p v-else-if="!suggestion && !error" class="muted">{{ s.none }}</p>
    <template v-if="suggestion">
      <SuggestionCard :suggestion="suggestion" :lang="target.code" :dir="target.direction" />
      <p v-if="suggestion.decision?.edit" class="hint">{{ s.editedBy(suggestion.decision.edit.distance) }}</p>
      <template v-if="pending && canDecide">
        <div v-if="editing" class="stack-sm">
          <label for="ai-edit" class="label">{{ s.editLabel }}</label>
          <textarea
            id="ai-edit"
            ref="editArea"
            v-model="editText"
            rows="3"
            class="mono"
            :lang="target.code"
            :dir="target.direction"
            aria-describedby="ai-edit-hint"
            data-testid="suggestion-edit"
            @keydown="onEditKey"
          />
          <span id="ai-edit-hint" class="hint">{{ s.editHint(`${keyLabel("Mod")}${keyLabel("Enter").split(" ")[0]}`) }}</span>
          <div class="row">
            <button type="button" class="btn btn-primary btn-sm" :disabled="busy || !editText.trim()" @click="acceptEdit">{{ s.acceptEdit }}</button>
            <button type="button" class="btn btn-sm" :disabled="busy" @click="editing = false">{{ strings.app.cancel }}</button>
          </div>
        </div>
        <form v-else-if="rejecting" class="stack-sm" @submit.prevent="reject">
          <label for="ai-reason" class="label">{{ s.reasonLabel }}</label>
          <input id="ai-reason" v-model="reason" maxlength="2000" autocomplete="off" />
          <div class="row">
            <button type="submit" class="btn btn-danger btn-sm" :disabled="busy">{{ s.reject }}</button>
            <button type="button" class="btn btn-sm" :disabled="busy" @click="rejecting = false">{{ strings.app.cancel }}</button>
          </div>
        </form>
        <div v-else class="row">
          <button type="button" class="btn btn-primary btn-sm" :disabled="busy" :aria-keyshortcuts="ariaKeys(['Mod', 'Alt', 'Enter'])" @click="accept">
            {{ s.accept }} <span class="kbd-hint" aria-hidden="true">{{ keyLabel("Mod") }}{{ keyLabel("Alt") }}↵</span>
          </button>
          <button type="button" class="btn btn-sm" :disabled="busy" :aria-keyshortcuts="ariaKeys(['Mod', 'Alt', '0'])" @click="startEdit">
            {{ s.edit }} <span class="kbd-hint" aria-hidden="true">{{ keyLabel("Mod") }}{{ keyLabel("Alt") }}0</span>
          </button>
          <button type="button" class="btn btn-danger btn-sm" :disabled="busy" @click="rejecting = true">{{ s.rejectEllipsis }}</button>
        </div>
      </template>
      <p v-else-if="pending" class="hint">{{ s.noDecide(target.code) }}</p>
    </template>
    <p class="notice" role="status" data-testid="ai-status">{{ notice }}</p>
  </section>
</template>

<style scoped>
.notice {
  color: var(--gs-ok);
  font-weight: var(--kl-weight-medium);
}
textarea {
  inline-size: 100%;
}
</style>
