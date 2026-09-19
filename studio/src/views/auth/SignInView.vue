<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { RouterLink, useRoute, useRouter } from "vue-router";
import { auth } from "../../api/endpoints";
import { isApiError } from "../../api/errors";
import AuthLayout from "../../components/AuthLayout.vue";
import ErrorAlert from "../../components/ErrorAlert.vue";
import { forgetHash, safeNext, tokenFromHash } from "../../lib/links";
import { getPref, PASSKEY_EMAIL, PASSKEYS_DISABLED, setPref } from "../../lib/prefs";
import { getAssertion, passkeysSupported } from "../../lib/webauthn";
import { establishSession } from "../../session/session";
import { strings } from "../../strings";

const route = useRoute();
const router = useRouter();

const passkeyEmail = getPref(PASSKEY_EMAIL);
const email = ref(passkeyEmail ?? "");
const password = ref("");
const totp = ref("");
const needTotp = ref(false);
const busy = ref<"" | "link" | "password" | "passkey" | "redeem">("");
const error = ref<unknown>(null);
const sentTo = ref<string>();

const canPasskey = passkeysSupported() && !getPref(PASSKEYS_DISABLED);
const passkeyFirst = computed(() => canPasskey && passkeyEmail !== undefined && email.value === passkeyEmail);

async function finish(session: Parameters<typeof establishSession>[0]): Promise<void> {
  await establishSession(session);
  await router.replace(safeNext(route.query.next) ?? { name: "home" });
}

async function sendLink(): Promise<void> {
  error.value = null;
  busy.value = "link";
  try {
    await auth.requestMagicLink(email.value.trim());
    sentTo.value = email.value.trim();
  } catch (e) {
    error.value = e;
  } finally {
    busy.value = "";
  }
}

async function withPassword(): Promise<void> {
  error.value = null;
  busy.value = "password";
  try {
    const body = { email: email.value.trim(), password: password.value, ...(needTotp.value && totp.value ? { totp_code: totp.value } : {}) };
    await finish(await auth.signInWithPassword(body));
  } catch (e) {
    if (isApiError(e, "totp_required")) needTotp.value = true;
    error.value = e;
  } finally {
    busy.value = "";
  }
}

async function withPasskey(): Promise<void> {
  error.value = null;
  busy.value = "passkey";
  try {
    const { options } = await auth.beginPasskeySignIn(email.value.trim());
    const session = await auth.finishPasskeySignIn(await getAssertion(options));
    setPref(PASSKEY_EMAIL, email.value.trim());
    await finish(session);
  } catch (e) {
    if (isApiError(e, "passkeys_disabled")) setPref(PASSKEYS_DISABLED, "1");
    error.value = e;
  } finally {
    busy.value = "";
  }
}

onMounted(async () => {
  const token = tokenFromHash(location.hash);
  if (!token) return;
  forgetHash();
  busy.value = "redeem";
  try {
    await finish(await auth.redeemMagicLink(token));
  } catch (e) {
    error.value = isApiError(e, "link_invalid") ? new Error(strings.auth.linkInvalid) : e;
  } finally {
    busy.value = "";
  }
});
</script>

<template>
  <AuthLayout v-if="busy === 'redeem'" :title="strings.auth.redeeming">
    <p role="status" class="muted">{{ strings.app.loading }}</p>
  </AuthLayout>

  <AuthLayout v-else-if="sentTo" :title="strings.auth.linkSentTitle">
    <p role="status">{{ strings.auth.linkSent(sentTo) }}</p>
    <button type="button" class="btn" @click="sentTo = undefined">{{ strings.auth.useDifferentEmail }}</button>
  </AuthLayout>

  <AuthLayout v-else :title="strings.auth.signInTitle" :lead="strings.auth.signInLead">
    <ErrorAlert :error="error" />
    <form class="stack" @submit.prevent="passkeyFirst ? withPasskey() : sendLink()">
      <div class="field">
        <label for="email">{{ strings.auth.email }}</label>
        <input id="email" v-model="email" type="email" name="email" autocomplete="username webauthn" required />
      </div>
      <template v-if="passkeyFirst">
        <button type="submit" class="btn btn-primary" :disabled="busy !== ''">{{ strings.auth.passkeyPrimary(email) }}</button>
        <button type="button" class="btn" :disabled="busy !== '' || !email" @click="sendLink">
          {{ busy === "link" ? strings.auth.sending : strings.auth.sendLink }}
        </button>
      </template>
      <button v-else type="submit" class="btn btn-primary" :disabled="busy !== ''">
        {{ busy === "link" ? strings.auth.sending : strings.auth.sendLink }}
      </button>
    </form>

    <details class="others">
      <summary>{{ strings.auth.otherMethods }}</summary>
      <div class="stack others-body">
        <button v-if="canPasskey && !passkeyFirst" type="button" class="btn" :disabled="busy !== '' || !email" @click="withPasskey">
          {{ strings.auth.passkey }}
        </button>
        <form class="stack" @submit.prevent="withPassword">
          <div class="field">
            <label for="password">{{ strings.auth.password }}</label>
            <input id="password" v-model="password" type="password" name="password" autocomplete="current-password" required />
          </div>
          <div v-if="needTotp" class="field">
            <label for="totp">{{ strings.auth.totp }}</label>
            <input id="totp" v-model="totp" inputmode="numeric" pattern="[0-9]{6}" maxlength="6" autocomplete="one-time-code" aria-describedby="totp-hint" required />
            <span id="totp-hint" class="hint">{{ strings.auth.totpHint }}</span>
          </div>
          <button type="submit" class="btn" :disabled="busy !== '' || !email">{{ strings.auth.signInWithPassword }}</button>
        </form>
        <RouterLink :to="{ name: 'reset-password' }">{{ strings.auth.forgotPassword }}</RouterLink>
      </div>
    </details>

    <template #footer>
      {{ strings.auth.noAccount }} <RouterLink :to="{ name: 'register' }">{{ strings.auth.register }}</RouterLink>
    </template>
  </AuthLayout>
</template>

<style scoped>
.others summary {
  cursor: pointer;
  color: var(--kl-ink-secondary);
}
.others-body {
  margin-block-start: var(--kl-space-4);
}
</style>
