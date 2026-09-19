<script setup lang="ts">
import { computed, provide, ref, watch } from "vue";
import { RouterLink, RouterView, useRoute } from "vue-router";
import { locales as localesApi, projects } from "../../api/endpoints";
import type { Project, ProjectLocale } from "../../api/schemas";
import ErrorAlert from "../../components/ErrorAlert.vue";
import { useGrant } from "../../session/session";
import { strings } from "../../strings";
import { PROJECT, type ProjectContext } from "./context";

const route = useRoute();
const tenant = computed(() => String(route.params.tenant));
const projectId = computed(() => String(route.params.project));
const project = ref<Project>();
const etag = ref<string>();
const locales = ref<ProjectLocale[]>([]);
const error = ref<unknown>(null);
const grant = useGrant(() => tenant.value);

async function reloadProject(): Promise<void> {
  const r = await projects.get({ tenant: tenant.value, project: projectId.value });
  project.value = r.value;
  etag.value = r.etag;
}
async function reloadLocales(): Promise<void> {
  locales.value = await localesApi.list({ tenant: tenant.value, project: projectId.value });
}

watch(
  [tenant, projectId],
  async () => {
    error.value = null;
    project.value = undefined;
    try {
      await Promise.all([reloadProject(), reloadLocales()]);
    } catch (e) {
      error.value = e;
    }
  },
  { immediate: true },
);

const ctx: ProjectContext = {
  tenant,
  projectId,
  project,
  etag,
  locales,
  targets: computed(() => locales.value.filter((l) => !l.is_source)),
  grant,
  reloadProject,
  reloadLocales,
};
provide(PROJECT, ctx);

const tabs = [
  { name: "translate", label: strings.nav.translate },
  { name: "review", label: strings.nav.review },
  { name: "terms", label: strings.nav.termbase },
  { name: "style", label: strings.nav.style },
  { name: "locales", label: strings.nav.locales },
  { name: "releases", label: strings.nav.releases },
  { name: "settings", label: strings.nav.settings },
] as const;
</script>

<template>
  <div class="project-layout">
    <div class="project-bar">
      <nav :aria-label="strings.nav.breadcrumb" class="crumbs">
        <RouterLink :to="{ name: 'projects', params: { tenant } }">{{ strings.nav.projects }}</RouterLink>
        <span aria-hidden="true">/</span>
        <span class="current" aria-current="page">{{ project?.name ?? "…" }}</span>
      </nav>
      <nav :aria-label="strings.nav.projectSections" class="tabs">
        <RouterLink
          v-for="t in tabs"
          :key="t.name"
          :to="{ name: t.name, params: { tenant, project: projectId } }"
          class="tab"
          :class="{ 'router-link-active': t.name === 'releases' && route.name === 'release' }"
        >
          {{ t.label }}
        </RouterLink>
      </nav>
    </div>
    <div v-if="error" class="page"><ErrorAlert :error="error" /></div>
    <RouterView v-else-if="project" />
    <p v-else class="page muted" role="status">{{ strings.app.loading }}</p>
  </div>
</template>

<style scoped>
.project-layout {
  flex: 1;
  display: flex;
  flex-direction: column;
  min-block-size: 0;
}
.project-bar {
  display: flex;
  align-items: center;
  gap: var(--kl-space-6);
  padding: 0 var(--kl-space-4);
  border-block-end: 1px solid var(--kl-border);
  flex-wrap: wrap;
}
.crumbs {
  display: flex;
  gap: var(--kl-space-2);
  align-items: center;
  padding-block: var(--kl-space-3);
}
.crumbs a {
  color: var(--kl-ink-secondary);
}
.current {
  font-weight: var(--kl-weight-semibold);
}
.tabs {
  display: flex;
  gap: var(--kl-space-1);
}
.tab {
  padding: var(--kl-space-3) var(--kl-space-3);
  color: var(--kl-ink-secondary);
  text-decoration: none;
  border-block-end: 2px solid transparent;
  margin-block-end: -1px;
}
.tab:hover {
  color: var(--kl-ink);
}
.tab.router-link-active {
  color: var(--kl-ink);
  border-block-end-color: var(--kl-accent);
  font-weight: var(--kl-weight-medium);
}
</style>
