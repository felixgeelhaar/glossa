<script setup lang="ts">
import { computed, inject, ref, watch } from "vue";
import { comingSoonReleases, RELEASES, type ReleaseSummary } from "../../api/releases";
import ErrorAlert from "../../components/ErrorAlert.vue";
import { allows } from "../../session/permissions";
import { strings } from "../../strings";
import { useProject } from "./context";

const port = inject(RELEASES, comingSoonReleases);
const { tenant, projectId, grant } = useProject();
const s = strings.releases;
const available = port.availability === "available";
const canPublish = computed(() => allows(grant.value, "releases.publish"));

const list = ref<ReleaseSummary[]>([]);
const error = ref<unknown>(null);
const busy = ref(false);

async function load(): Promise<void> {
  if (!available) return;
  try {
    list.value = await port.list({ tenant: tenant.value, project: projectId.value });
  } catch (e) {
    error.value = e;
  }
}
watch(projectId, load, { immediate: true });

async function publish(): Promise<void> {
  busy.value = true;
  error.value = null;
  try {
    await port.publish({ tenant: tenant.value, project: projectId.value });
    await load();
  } catch (e) {
    error.value = e;
  } finally {
    busy.value = false;
  }
}
</script>

<template>
  <div class="page stack">
    <h1>{{ s.title }}</h1>
    <section v-if="!available" class="card stack soon" aria-labelledby="soon-h" data-testid="releases-coming-soon">
      <div class="row">
        <h2 id="soon-h">{{ s.comingSoonTitle }}</h2>
        <span class="pill pill-accent">{{ s.comingSoon }}</span>
      </div>
      <p>{{ s.comingSoonLead }}</p>
      <ul>
        <li v-for="item in s.planned" :key="item">{{ item }}</li>
      </ul>
      <p class="muted">{{ s.meanwhile }}</p>
    </section>
    <template v-else>
      <ErrorAlert :error="error" />
      <div v-if="canPublish"><button type="button" class="btn btn-primary" :disabled="busy" @click="publish">{{ s.publish }}</button></div>
      <table class="table">
        <thead>
          <tr>
            <th scope="col">{{ s.columns.release }}</th>
            <th scope="col">{{ s.columns.created }}</th>
            <th scope="col">{{ s.columns.environments }}</th>
            <th scope="col">{{ s.columns.locales }}</th>
            <th scope="col">{{ s.columns.messages }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="r in list" :key="r.id">
            <td>{{ r.name }}</td>
            <td><time :datetime="r.createdAt">{{ new Date(r.createdAt).toLocaleString() }}</time></td>
            <td>{{ r.environments.join(", ") }}</td>
            <td>{{ r.locales }}</td>
            <td>{{ r.messages }}</td>
          </tr>
        </tbody>
      </table>
    </template>
  </div>
</template>

<style scoped>
.soon {
  max-inline-size: var(--kl-content-md);
}
.soon ul {
  margin: 0;
  padding-inline-start: var(--kl-space-5);
}
</style>
