<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { RouterLink, useRoute, useRouter } from "vue-router";
import { projects as projectsApi } from "../api/endpoints";
import type { Project } from "../api/schemas";
import ErrorAlert from "../components/ErrorAlert.vue";
import LocaleInput from "../components/LocaleInput.vue";
import { checkLocale } from "../lib/bcp47";
import { SLUG_PATTERN, slugify } from "../lib/slug";
import { allows } from "../session/permissions";
import { membershipFor, useGrant } from "../session/session";
import { strings } from "../strings";

const route = useRoute();
const router = useRouter();
const tenant = computed(() => String(route.params.tenant));
const grant = useGrant(() => tenant.value);
const canCreate = computed(() => allows(grant.value, "catalog.write"));
const tenantName = computed(() => membershipFor(tenant.value)?.tenant.name ?? "");

const list = ref<Project[]>([]);
const loading = ref(true);
const loadError = ref<unknown>(null);

async function load(): Promise<void> {
  loading.value = true;
  loadError.value = null;
  try {
    list.value = await projectsApi.list(tenant.value);
  } catch (e) {
    loadError.value = e;
  } finally {
    loading.value = false;
  }
}
watch(tenant, load, { immediate: true });

const creating = ref(false);
const name = ref("");
const slug = ref("");
const slugTouched = ref(false);
const sourceLocale = ref("en");
const syntax = ref<"mf1" | "mf2">("mf1");
const reviewRequired = ref(true);
const busy = ref(false);
const createError = ref<unknown>(null);
watch(name, (n) => {
  if (!slugTouched.value) slug.value = slugify(n);
});

async function create(): Promise<void> {
  const locale = checkLocale(sourceLocale.value);
  if (!locale.ok) return;
  createError.value = null;
  busy.value = true;
  try {
    const { value: p } = await projectsApi.create(tenant.value, {
      name: name.value.trim(),
      slug: slug.value,
      source_locale: locale.tag,
      settings: { default_syntax: syntax.value, review_required: reviewRequired.value },
    });
    await router.push({ name: "locales", params: { tenant: tenant.value, project: p.id } });
  } catch (e) {
    createError.value = e;
  } finally {
    busy.value = false;
  }
}
</script>

<template>
  <div class="page">
    <div class="page-header">
      <div class="stack-sm">
        <p class="muted">{{ tenantName }}</p>
        <h1>{{ strings.projects.title }}</h1>
      </div>
      <button v-if="canCreate && !creating" type="button" class="btn btn-primary" @click="creating = true">{{ strings.projects.create }}</button>
    </div>

    <section v-if="creating" class="card stack create" aria-labelledby="create-h">
      <h2 id="create-h">{{ strings.projects.createTitle }}</h2>
      <ErrorAlert :error="createError" />
      <form class="grid" @submit.prevent="create">
        <div class="field">
          <label for="p-name">{{ strings.projects.name }}</label>
          <input id="p-name" v-model="name" maxlength="200" required />
        </div>
        <div class="field">
          <label for="p-slug">{{ strings.projects.slug }}</label>
          <input id="p-slug" v-model="slug" class="mono" :pattern="SLUG_PATTERN" maxlength="63" required aria-describedby="p-slug-hint" @input="slugTouched = true" />
          <span id="p-slug-hint" class="hint">{{ strings.tenants.slugHint }}</span>
        </div>
        <LocaleInput id="p-source" v-model="sourceLocale" :label="strings.projects.sourceLocale" :hint="strings.projects.sourceLocaleHint" required />
        <div class="field">
          <label for="p-syntax">{{ strings.projects.defaultSyntax }}</label>
          <select id="p-syntax" v-model="syntax">
            <option value="mf1">{{ strings.projects.syntaxMf1 }}</option>
            <option value="mf2">{{ strings.projects.syntaxMf2 }}</option>
          </select>
        </div>
        <label class="check wide">
          <input v-model="reviewRequired" type="checkbox" aria-describedby="p-review-hint" />
          <span class="stack-sm">
            <span>{{ strings.projects.reviewRequired }}</span>
            <span id="p-review-hint" class="hint">{{ strings.projects.reviewRequiredHint }}</span>
          </span>
        </label>
        <div class="row wide">
          <button type="submit" class="btn btn-primary" :disabled="busy">{{ strings.projects.submit }}</button>
          <button type="button" class="btn btn-ghost" @click="creating = false">{{ strings.app.cancel }}</button>
        </div>
      </form>
    </section>

    <ErrorAlert :error="loadError" />
    <p v-if="loading" role="status" class="muted">{{ strings.app.loading }}</p>
    <p v-else-if="!loadError && list.length === 0" class="card muted">{{ strings.projects.empty }}</p>
    <ul v-else class="projects" role="list">
      <li v-for="p in list" :key="p.id">
        <RouterLink class="project card" :to="{ name: 'translate', params: { tenant, project: p.id } }">
          <span class="p-name">{{ p.name }}</span>
          <span class="mono muted">{{ p.slug }}</span>
          <span class="pill pill-neutral">{{ strings.projects.source(p.source_locale) }}</span>
        </RouterLink>
      </li>
    </ul>
  </div>
</template>

<style scoped>
.create {
  margin-block-end: var(--kl-space-6);
}
.grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(16rem, 1fr));
  gap: var(--kl-space-4);
}
.wide {
  grid-column: 1 / -1;
}
.projects {
  list-style: none;
  padding: 0;
  margin: 0;
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(18rem, 1fr));
  gap: var(--kl-space-4);
}
.project {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: var(--kl-space-2);
  text-decoration: none;
  color: var(--kl-ink);
  transition: border-color var(--kl-duration-fast) var(--kl-ease-default);
}
.project:hover {
  border-color: var(--kl-accent-border);
}
.p-name {
  font-weight: var(--kl-weight-semibold);
  font-size: var(--kl-text-md);
}
</style>
