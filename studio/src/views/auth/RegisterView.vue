<script setup lang="ts">
import { ref } from "vue";
import { RouterLink } from "vue-router";
import { auth } from "../../api/endpoints";
import AuthLayout from "../../components/AuthLayout.vue";
import ErrorAlert from "../../components/ErrorAlert.vue";
import { strings } from "../../strings";

const email = ref("");
const name = ref("");
const password = ref("");
const busy = ref(false);
const error = ref<unknown>(null);
const sentTo = ref<string>();

async function submit(): Promise<void> {
  error.value = null;
  busy.value = true;
  try {
    const display_name = name.value.trim();
    await auth.register({ email: email.value.trim(), password: password.value, ...(display_name ? { display_name } : {}) });
    sentTo.value = email.value.trim();
  } catch (e) {
    error.value = e;
  } finally {
    busy.value = false;
  }
}
</script>

<template>
  <AuthLayout v-if="sentTo" :title="strings.auth.linkSentTitle">
    <p role="status">{{ strings.auth.registered(sentTo) }}</p>
    <RouterLink :to="{ name: 'sign-in' }">{{ strings.auth.backToSignIn }}</RouterLink>
  </AuthLayout>
  <AuthLayout v-else :title="strings.auth.registerTitle" :lead="strings.auth.registerLead">
    <ErrorAlert :error="error" />
    <form class="stack" @submit.prevent="submit">
      <div class="field">
        <label for="email">{{ strings.auth.email }}</label>
        <input id="email" v-model="email" type="email" autocomplete="email" required />
      </div>
      <div class="field">
        <label for="name">{{ strings.auth.displayName }}</label>
        <input id="name" v-model="name" autocomplete="name" maxlength="200" />
      </div>
      <div class="field">
        <label for="password">{{ strings.auth.newPassword }}</label>
        <input id="password" v-model="password" type="password" autocomplete="new-password" minlength="12" maxlength="1024" required />
      </div>
      <button type="submit" class="btn btn-primary" :disabled="busy">{{ strings.auth.registerSubmit }}</button>
    </form>
    <template #footer>
      {{ strings.auth.haveAccount }} <RouterLink :to="{ name: 'sign-in' }">{{ strings.auth.backToSignIn }}</RouterLink>
    </template>
  </AuthLayout>
</template>
