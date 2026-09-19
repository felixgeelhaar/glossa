<script setup lang="ts">
import { ref } from "vue";
import { useRouter } from "vue-router";
import { me as meApi } from "../api/endpoints";
import { isApiError } from "../api/errors";
import type { TotpEnrollment } from "../api/schemas";
import ErrorAlert from "../components/ErrorAlert.vue";
import { PASSKEY_EMAIL, PASSKEYS_DISABLED, setPref } from "../lib/prefs";
import { createCredential, passkeysSupported } from "../lib/webauthn";
import { refreshSession, signOut, useSession } from "../session/session";
import { strings } from "../strings";

const router = useRouter();
const { person } = useSession();
const s = strings.account;

const supported = passkeysSupported();
const passkeyName = ref<string>(s.passkeyNameDefault);
const passkeyBusy = ref(false);
const passkeyError = ref<unknown>(null);
const passkeyDone = ref("");

async function addPasskey(): Promise<void> {
  passkeyError.value = null;
  passkeyDone.value = "";
  passkeyBusy.value = true;
  try {
    const { options } = await meApi.beginPasskey();
    const pk = await meApi.finishPasskey(passkeyName.value.trim() || s.passkeyNameDefault, await createCredential(options));
    if (person.value) setPref(PASSKEY_EMAIL, person.value.email);
    passkeyDone.value = s.passkeyAdded(pk.name);
  } catch (e) {
    if (isApiError(e, "passkeys_disabled")) setPref(PASSKEYS_DISABLED, "1");
    passkeyError.value = e;
  } finally {
    passkeyBusy.value = false;
  }
}

const enrollment = ref<TotpEnrollment>();
const code = ref("");
const totpBusy = ref(false);
const totpError = ref<unknown>(null);
const totpDone = ref("");

async function totpStep(action: () => Promise<void>): Promise<void> {
  totpError.value = null;
  totpDone.value = "";
  totpBusy.value = true;
  try {
    await action();
  } catch (e) {
    totpError.value = e;
  } finally {
    totpBusy.value = false;
  }
}
const startTotp = () => totpStep(async () => void (enrollment.value = await meApi.beginTotp()));
const confirmTotp = () =>
  totpStep(async () => {
    await meApi.confirmTotp(code.value);
    enrollment.value = undefined;
    code.value = "";
    await refreshSession();
    totpDone.value = s.totpEnabled;
  });
const disableTotp = () =>
  totpStep(async () => {
    await meApi.disableTotp(code.value);
    code.value = "";
    await refreshSession();
    totpDone.value = s.totpDisabled;
  });

async function everywhere(): Promise<void> {
  await signOut(true).catch(() => undefined);
  await router.push({ name: "sign-in" });
}
</script>

<template>
  <div class="page stack">
    <h1>{{ s.title }}</h1>

    <section class="card stack" aria-labelledby="profile-h">
      <h2 id="profile-h">{{ s.profile }}</h2>
      <p>
        <strong>{{ person?.display_name || person?.email }}</strong>
        <span v-if="person?.display_name" class="muted"> · {{ person.email }}</span>
        <span class="pill" :class="person?.email_verified ? 'pill-ok' : 'pill-warn'">
          {{ person?.email_verified ? s.emailVerified : s.emailUnverified }}
        </span>
      </p>
    </section>

    <section id="passkeys" class="card stack" aria-labelledby="passkeys-h">
      <h2 id="passkeys-h">{{ s.passkeys }}</h2>
      <p class="muted">{{ s.passkeysLead }}</p>
      <p v-if="!supported" class="alert alert-warn">{{ s.passkeysUnsupported }}</p>
      <form v-else class="row" @submit.prevent="addPasskey">
        <div class="field">
          <label for="passkey-name">{{ s.passkeyName }}</label>
          <input id="passkey-name" v-model="passkeyName" maxlength="100" />
        </div>
        <button type="submit" class="btn btn-primary add" :disabled="passkeyBusy">{{ s.addPasskey }}</button>
      </form>
      <p v-if="passkeyDone" class="alert alert-ok" role="status">{{ passkeyDone }}</p>
      <ErrorAlert :error="passkeyError" />
    </section>

    <section class="card stack" aria-labelledby="totp-h">
      <div class="row">
        <h2 id="totp-h">{{ s.totp }}</h2>
        <span class="pill" :class="person?.totp_enabled ? 'pill-ok' : 'pill-neutral'">{{ person?.totp_enabled ? s.totpOn : s.totpOff }}</span>
      </div>
      <p class="muted">{{ s.totpLead }}</p>
      <template v-if="enrollment">
        <p>{{ s.totpSecret }}</p>
        <p><code class="secret">{{ enrollment.secret }}</code></p>
        <p><a :href="enrollment.otpauth_uri">{{ s.totpOpen }}</a></p>
      </template>
      <form v-if="enrollment || person?.totp_enabled" class="row" @submit.prevent="enrollment ? confirmTotp() : disableTotp()">
        <div class="field">
          <label for="totp-code">{{ strings.auth.totp }}</label>
          <input id="totp-code" v-model="code" inputmode="numeric" pattern="[0-9]{6}" maxlength="6" autocomplete="one-time-code" required />
        </div>
        <button type="submit" class="btn add" :class="enrollment ? 'btn-primary' : 'btn-danger'" :disabled="totpBusy">
          {{ enrollment ? s.totpConfirm : s.totpDisable }}
        </button>
      </form>
      <div v-else>
        <button type="button" class="btn" :disabled="totpBusy" @click="startTotp">{{ s.totpStart }}</button>
      </div>
      <p v-if="totpDone" class="alert alert-ok" role="status">{{ totpDone }}</p>
      <ErrorAlert :error="totpError" />
    </section>

    <section class="card stack" aria-labelledby="sessions-h">
      <h2 id="sessions-h">{{ s.sessions }}</h2>
      <p class="muted">{{ s.sessionsLead }}</p>
      <div><button type="button" class="btn btn-danger" @click="everywhere">{{ strings.nav.signOutEverywhere }}</button></div>
    </section>
  </div>
</template>

<style scoped>
.page {
  max-inline-size: var(--kl-content-md);
}
.add {
  align-self: flex-end;
}
.secret {
  word-break: break-all;
  font-size: var(--kl-text-base);
}
</style>
