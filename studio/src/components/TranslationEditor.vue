<script setup lang="ts">
/**
 * One message in one target locale: context, source with its arguments,
 * the translation editor with live preview, structural QA from the
 * server, review actions and the revision history.
 */
import type { Message as MF2 } from "@glossa/messageformat";
import { computed, ref, shallowRef, useTemplateRef, watch } from "vue";
import { messages as messagesApi, translations } from "../api/endpoints";
import { ApiError, isApiError } from "../api/errors";
import type { Message, Project, ProjectLocale, QAFinding, ReviewState, SourceRevision, Syntax, Translation, TranslationRevision } from "../api/schemas";
import { loadFormatter, parseMF2 } from "../lib/preview";
import { reviewActions, stateTone } from "../lib/review";
import { keyLabel } from "../lib/shortcuts";
import { allowsFor, type Grant } from "../session/permissions";
import { problemText, strings } from "../strings";
import HistoryList from "./HistoryList.vue";
import PreviewPanel from "./PreviewPanel.vue";
import QaFindings from "./QaFindings.vue";
import SourceDiff from "./SourceDiff.vue";

const props = defineProps<{
  tenant: string;
  project: Project;
  message: Message;
  locale: ProjectLocale;
  source: ProjectLocale;
  grant: Grant;
  selfId?: string | undefined;
}>();
const emit = defineEmits<{ changed: [translation: Translation] }>();
const s = strings.workspace;

const path = computed(() => ({ tenant: props.tenant, project: props.project.id, message: props.message.key, locale: props.locale.code }));
const canWrite = computed(() => allowsFor(props.grant, "translations.write", props.locale.code));
const canReview = computed(() => allowsFor(props.grant, "translations.review", props.locale.code));

const translation = ref<Translation | null>(null);
const etag = ref<string>();
const loading = ref(false);
const loadError = ref<unknown>(null);
const draft = ref("");
const syntax = ref<Syntax>("mf1");
const findings = ref<QAFinding[]>([]);
const parseError = ref("");
const actionError = ref<unknown>(null);
const notice = ref("");
const busy = ref(false);
const tab = ref<"editor" | "history">("editor");
const revisions = ref<TranslationRevision[]>([]);
const sourceRevisions = ref<SourceRevision[]>();
const showDiff = ref(false);
const area = useTemplateRef<HTMLTextAreaElement>("area");

const dirty = computed(() => draft.value !== (translation.value?.text ?? ""));

let loadSeq = 0;
async function load(): Promise<void> {
  const seq = ++loadSeq;
  loading.value = true;
  loadError.value = null;
  resetFeedback();
  showDiff.value = false;
  sourceRevisions.value = undefined;
  revisions.value = [];
  try {
    const r = await translations.get(path.value);
    if (seq !== loadSeq) return;
    adopt(r?.value ?? null, r?.etag);
    draft.value = r?.value.text ?? "";
    if (tab.value === "history") await loadHistory();
  } catch (e) {
    if (seq === loadSeq) loadError.value = e;
  } finally {
    if (seq === loadSeq) loading.value = false;
  }
}
watch(path, load, { immediate: true });

function adopt(t: Translation | null, tag: string | undefined): void {
  translation.value = t;
  etag.value = tag;
  syntax.value = t?.syntax ?? props.project.settings.default_syntax;
}

function resetFeedback(): void {
  findings.value = [];
  parseError.value = "";
  actionError.value = null;
  notice.value = "";
}

async function loadHistory(): Promise<void> {
  if (!translation.value) {
    revisions.value = [];
    return;
  }
  try {
    revisions.value = await translations.revisions(path.value);
  } catch (e) {
    actionError.value = e;
  }
}
watch(tab, (t) => {
  if (t === "history") void loadHistory();
});

async function toggleDiff(): Promise<void> {
  showDiff.value = !showDiff.value;
  if (showDiff.value && !sourceRevisions.value) {
    try {
      sourceRevisions.value = await messagesApi.sourceRevisions({ tenant: props.tenant, project: props.project.id, message: props.message.key });
    } catch (e) {
      actionError.value = e;
    }
  }
}
const oldSource = computed(() => sourceRevisions.value?.find((r) => r.revision === translation.value?.source_revision)?.text);

/** Save the draft; with `approve`, approve it too (a reviewer's one-step accept). */
async function save(approve = false): Promise<void> {
  if (busy.value || loading.value) return;
  if (!canWrite.value) {
    actionError.value = new Error(s.noWrite(props.locale.code));
    return;
  }
  if (!dirty.value) {
    if (approve && translation.value && translation.value.state !== "approved") return review("approved");
    notice.value = s.unchanged;
    return;
  }
  resetFeedback();
  busy.value = true;
  try {
    const body = { text: draft.value, syntax: syntax.value, ...(approve ? { state: "approved" as const } : {}) };
    const r = await translations.put(path.value, body, etag.value);
    adopt(r.value, r.etag);
    draft.value = r.value.text;
    notice.value = approve ? s.approved : s.saved;
    emit("changed", r.value);
    if (tab.value === "history") await loadHistory();
  } catch (e) {
    await handleWriteError(e);
  } finally {
    busy.value = false;
  }
}

async function review(state: ReviewState): Promise<void> {
  if (busy.value || !translation.value || !etag.value) return;
  resetFeedback();
  busy.value = true;
  try {
    const r = await translations.review(path.value, state, etag.value);
    adopt(r.value, r.etag);
    notice.value = state === "approved" ? s.approved : state === "rejected" ? s.rejected : s.saved;
    emit("changed", r.value);
    if (tab.value === "history") await loadHistory();
  } catch (e) {
    await handleWriteError(e);
  } finally {
    busy.value = false;
  }
}

async function handleWriteError(e: unknown): Promise<void> {
  if (isApiError(e, "structural_qa_failed")) {
    findings.value = (e as ApiError).findings;
    area.value?.focus();
    return;
  }
  if (isApiError(e, "invalid_message")) {
    parseError.value = (e as ApiError).message;
    area.value?.focus();
    return;
  }
  if (isApiError(e, "precondition_failed") || isApiError(e, "translation_conflict")) {
    const kept = draft.value;
    const r = await translations.get(path.value).catch(() => null);
    adopt(r?.value ?? null, r?.etag);
    draft.value = kept;
    actionError.value = new Error(s.conflict);
    return;
  }
  actionError.value = e;
}

// ── live preview ────────────────────────────────────────────────────
const liveModel = shallowRef<MF2>();
let parseTimer: ReturnType<typeof setTimeout> | undefined;
watch(
  [draft, syntax],
  () => {
    clearTimeout(parseTimer);
    if (syntax.value !== "mf2") {
      // A server parse error refers to text that has since changed.
      parseError.value = "";
      return;
    }
    parseTimer = setTimeout(async () => {
      const mf = await loadFormatter();
      const r = draft.value.trim() === "" ? undefined : parseMF2(mf, draft.value);
      liveModel.value = r?.ok ? r.model : undefined;
      parseError.value = r && !r.ok ? r.error : "";
    }, 120);
  },
  { immediate: true },
);
const targetModel = computed<MF2 | undefined>(() => {
  if (syntax.value === "mf2") return liveModel.value;
  return translation.value?.model as MF2 | undefined;
});
const previewNotice = computed(() => (syntax.value === "mf1" && (dirty.value || !translation.value) ? s.previewMf1Stale : undefined));

const actions = computed(() => reviewActions(translation.value?.state, canWrite.value, canReview.value));
const errorIds = computed(() => [findings.value.length ? "qa-errors" : "", parseError.value ? "parse-error" : "", "target-hint"].filter(Boolean).join(" "));

/** The source as canonical MF2 syntax — what releases ship — for developers. */
const canonical = ref("");
async function loadCanonical(): Promise<void> {
  if (canonical.value) return;
  const mf = await loadFormatter();
  try {
    canonical.value = mf.stringify(props.message.source.model as unknown as MF2);
  } catch (e) {
    canonical.value = e instanceof Error ? e.message : String(e);
  }
}

const markupLabel = (m: { name: string; kind: string }) => (m.kind === "close" ? `{/${m.name}}` : m.kind === "standalone" ? `{#${m.name} /}` : `{#${m.name}}`);

function focusEditor(): void {
  tab.value = "editor";
  area.value?.focus();
}
function blurEditor(): void {
  area.value?.blur();
}
defineExpose({ save, focusEditor, blurEditor, isEditing: () => document.activeElement === area.value });
</script>

<template>
  <article class="editor" :aria-labelledby="`msg-${message.id}`">
    <header class="head">
      <div class="title">
        <h2 :id="`msg-${message.id}`" class="mono key">{{ message.key }}</h2>
        <span class="pill pill-neutral">{{ message.namespace }}</span>
        <span v-if="message.state === 'obsolete'" class="pill pill-warn">{{ s.stateObsolete }}</span>
        <span v-if="message.max_length" class="pill pill-neutral">{{ s.maxLength(message.max_length) }}</span>
      </div>
      <div class="tabs" role="tablist" :aria-label="s.target">
        <button id="tab-editor" type="button" role="tab" :aria-selected="tab === 'editor'" aria-controls="panel-editor" class="tab" @click="tab = 'editor'">{{ s.tabs.editor }}</button>
        <button id="tab-history" type="button" role="tab" :aria-selected="tab === 'history'" aria-controls="panel-history" class="tab" @click="tab = 'history'">{{ s.tabs.history }}</button>
      </div>
    </header>

    <div v-show="tab === 'editor'" id="panel-editor" role="tabpanel" aria-labelledby="tab-editor" class="panel stack">
      <section class="context" :aria-label="s.description">
        <p v-if="message.description">{{ message.description }}</p>
        <p v-else class="muted">{{ s.noDescription }}</p>
      </section>

      <section class="stack-sm" aria-labelledby="source-h">
        <h3 id="source-h">{{ s.source }} <span class="muted mono">{{ source.code }}</span></h3>
        <p class="source-text" :lang="source.code" :dir="source.direction">{{ message.source.text }}</p>
        <ul class="chips" :aria-label="s.arguments">
          <li v-for="a in message.source.arguments" :key="a.name" class="chip">
            <code>${{ a.name }}</code>
            <span class="muted">{{ a.type }}</span>
            <span v-if="a.selector" class="muted">· {{ a.selector.kind }}: {{ a.selector.keys.join(" ") }}</span>
          </li>
          <li v-for="m in message.source.markup" :key="`m-${m.name}-${m.kind}`" class="chip chip-markup">
            <code>{{ markupLabel(m) }}</code>
            <span class="muted">{{ s.markup }}</span>
          </li>
          <li v-if="!message.source.arguments.length && !message.source.markup.length" class="muted">{{ s.noArguments }}</li>
        </ul>
        <details class="canonical" @toggle="loadCanonical">
          <summary class="hint">{{ s.canonical }}</summary>
          <pre class="mono">{{ canonical }}</pre>
        </details>
      </section>

      <section class="stack-sm target" aria-labelledby="target-h">
        <div class="row">
          <h3 id="target-h">{{ s.target }} <span class="muted mono">{{ locale.code }}</span></h3>
          <template v-if="translation">
            <span class="pill" :class="`pill-${stateTone(translation.state)}`" data-testid="translation-state">{{ s.stateLabel[translation.state] }}</span>
            <span class="muted">{{ s.origin }}: {{ translation.origin }}</span>
          </template>
          <span v-else-if="!loading" class="pill pill-err">{{ s.status.missing }}</span>
          <span v-if="dirty" class="pill pill-accent">{{ s.unsaved }}</span>
        </div>

        <div v-if="translation?.outdated" class="alert alert-warn stack-sm">
          <div class="row">
            <span class="alert-title">{{ s.outdated }}</span>
            <span>{{ s.outdatedLead(translation.source_revision, translation.current_source_revision) }}</span>
            <button type="button" class="btn btn-sm" :aria-expanded="showDiff" @click="toggleDiff">{{ showDiff ? s.hideSourceChanges : s.showSourceChanges }}</button>
          </div>
          <div v-if="showDiff && oldSource !== undefined" :aria-label="s.sourceChanges">
            <SourceDiff :before="oldSource" :after="message.source.text" :lang="source.code" :dir="source.direction" />
          </div>
        </div>

        <p v-if="loadError" class="alert alert-error" role="alert">{{ problemText(loadError) }}</p>
        <label for="target-text" class="visually-hidden">{{ s.target }} ({{ locale.code }})</label>
        <textarea
          id="target-text"
          ref="area"
          v-model="draft"
          class="target-text"
          rows="4"
          :lang="locale.code"
          :dir="locale.direction"
          :readonly="!canWrite"
          :disabled="loading"
          :aria-invalid="findings.length || parseError ? 'true' : undefined"
          :aria-describedby="errorIds"
          spellcheck="true"
          data-testid="target-editor"
        />
        <div class="row hint-row">
          <span id="target-hint" class="hint">
            <template v-if="!canWrite">{{ s.noWrite(locale.code) }}</template>
            <template v-else>{{ syntax === "mf1" ? strings.projects.syntaxMf1 : strings.projects.syntaxMf2 }}</template>
          </span>
          <span class="spacer" />
          <label class="hint" for="target-syntax">{{ s.syntax }}</label>
          <select id="target-syntax" v-model="syntax" class="syntax" :disabled="!canWrite">
            <option value="mf1">MF1</option>
            <option value="mf2">MF2</option>
          </select>
          <span v-if="message.max_length" class="hint" :class="{ over: draft.length > message.max_length }">{{ draft.length }} / {{ message.max_length }}</span>
        </div>

        <QaFindings v-if="findings.length" id="qa-errors" :title="s.qaFailed" :findings="findings" tone="error" data-testid="qa-findings" />
        <p v-if="parseError" id="parse-error" class="alert alert-error" role="alert">{{ s.invalidText }} <code>{{ parseError }}</code></p>
        <QaFindings v-if="translation?.warnings.length && !dirty" id="qa-warnings" :title="s.qaWarnings" :findings="translation.warnings" tone="warn" />

        <div class="row actions">
          <button type="button" class="btn btn-primary" :disabled="!canWrite || busy" @click="save(false)">
            {{ s.save }} <span class="kbd-hint">{{ keyLabel("Mod") }}{{ keyLabel("Enter").split(" ")[0] }}</span>
          </button>
          <button v-if="canReview" type="button" class="btn btn-ok" :disabled="!canWrite || busy" @click="save(true)">
            {{ s.saveApprove }} <span class="kbd-hint">{{ keyLabel("Mod") }}{{ keyLabel("Shift") }}{{ keyLabel("Enter").split(" ")[0] }}</span>
          </button>
          <span class="spacer" />
          <button v-if="!dirty && actions.includes('approve')" type="button" class="btn btn-ok" :disabled="busy" @click="review('approved')">{{ s.approve }}</button>
          <button v-if="!dirty && actions.includes('reject')" type="button" class="btn btn-danger" :disabled="busy" @click="review('rejected')">{{ s.reject }}</button>
          <button v-if="!dirty && actions.includes('request')" type="button" class="btn" :disabled="busy" @click="review('needs_review')">{{ s.requestReview }}</button>
        </div>
        <p v-if="!canReview && translation" class="hint">{{ s.noReview(locale.code) }}</p>
        <p class="notice" role="status" data-testid="editor-status">{{ notice }}</p>
        <p v-if="actionError" class="alert alert-error" role="alert">{{ problemText(actionError) }}</p>
      </section>

      <PreviewPanel
        :args="message.source.arguments"
        :source="{ model: message.source.model as unknown as MF2, locale: source.code, dir: source.direction }"
        :target="{ model: targetModel, locale: locale.code, dir: locale.direction }"
        :notice="previewNotice"
      />
    </div>

    <div v-show="tab === 'history'" id="panel-history" role="tabpanel" aria-labelledby="tab-history" class="panel">
      <HistoryList :revisions="revisions" :lang="locale.code" :dir="locale.direction" :self-id="selfId" />
    </div>
  </article>
</template>

<style scoped>
.editor {
  display: flex;
  flex-direction: column;
  min-block-size: 0;
}
.head {
  display: flex;
  justify-content: space-between;
  align-items: flex-end;
  gap: var(--kl-space-4);
  padding: var(--kl-space-4) var(--kl-space-6) 0;
  border-block-end: 1px solid var(--kl-border);
  flex-wrap: wrap;
}
.title {
  display: flex;
  align-items: center;
  gap: var(--kl-space-2);
  flex-wrap: wrap;
  padding-block-end: var(--kl-space-3);
}
.key {
  font-size: var(--kl-text-md);
  word-break: break-all;
}
.tabs {
  display: flex;
}
.tab {
  font: inherit;
  background: none;
  border: none;
  border-block-end: 2px solid transparent;
  padding: var(--kl-space-2) var(--kl-space-3);
  color: var(--kl-ink-secondary);
  cursor: pointer;
}
.tab[aria-selected="true"] {
  color: var(--kl-ink);
  border-block-end-color: var(--kl-accent);
}
.panel {
  padding: var(--kl-space-4) var(--kl-space-6) var(--kl-space-8);
}
.context {
  padding: var(--kl-space-3) var(--kl-space-4);
  border-inline-start: 3px solid var(--kl-border-strong);
  background: var(--kl-surface-raised);
  border-radius: 0 var(--kl-radius-md) var(--kl-radius-md) 0;
}
.source-text {
  font-size: var(--kl-text-md);
  white-space: pre-wrap;
  padding: var(--kl-space-3) var(--kl-space-4);
  background: var(--kl-surface-raised);
  border: 1px solid var(--kl-border);
  border-radius: var(--kl-radius-md);
}
.chips {
  list-style: none;
  padding: 0;
  margin: 0;
  display: flex;
  flex-wrap: wrap;
  gap: var(--kl-space-2);
}
.chip {
  display: inline-flex;
  gap: var(--kl-space-1);
  align-items: baseline;
  padding: 0.15em 0.6em;
  border-radius: var(--kl-radius-full);
  border: 1px solid var(--kl-accent-border);
  background: var(--kl-accent-dim);
  font-size: var(--kl-text-sm);
}
.chip-markup {
  border-color: var(--kl-border-strong);
  background: var(--kl-surface-muted);
}
.canonical pre {
  margin: var(--kl-space-2) 0 0;
  padding: var(--kl-space-3);
  background: var(--kl-surface-muted);
  border-radius: var(--kl-radius-md);
  white-space: pre-wrap;
  overflow-x: auto;
}
.canonical summary {
  cursor: pointer;
}
.target-text {
  inline-size: 100%;
  font-size: var(--kl-text-md);
  min-block-size: 7rem;
}
.hint-row {
  gap: var(--kl-space-2);
}
.syntax {
  min-block-size: 2rem;
  padding-block: 0;
}
.over {
  color: var(--gs-err);
  font-weight: var(--kl-weight-semibold);
}
.notice {
  color: var(--gs-ok);
  font-weight: var(--kl-weight-medium);
}
</style>
