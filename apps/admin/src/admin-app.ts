// <glossa-admin> — root of the SPA. Pragmatic MVP:
//
// - The backend JWT login flow is still future scope on
//   task-go-api, so for now we authenticate with a project-scoped
//   API key the translator pastes in. It's persisted to
//   localStorage so a refresh doesn't kick them out.
// - Translators pick a locale from a dropdown, browse the keys
//   with optional status filters, click one to edit, save, and
//   see the new value flow back through SSE into the demo strip
//   below so the live-update channel is obvious end to end.
//
// Bulk import / export, diff view, audit log, per-user access
// scoping all live in the spec but are explicitly deferred — this
// file ships the translator-edit golden path that the acceptance
// criterion calls out.

import { LitElement, css, html } from "lit";

import { createClient, type Bundle, type Client, type TranslationStatus } from "@glossa/sdk";

interface Settings {
  apiUrl: string;
  apiKey: string;
  project: string;
  locale: string;
}

const STORAGE_KEY = "glossa-admin-settings-v1";

export class GlossaAdmin extends LitElement {
  static override styles = css`
    :host {
      display: block;
      max-width: 960px;
      margin: 0 auto;
      padding: 24px;
    }
    header {
      display: flex;
      gap: 12px;
      align-items: center;
      flex-wrap: wrap;
      margin-bottom: 16px;
    }
    h1 {
      font-size: 18px;
      margin: 0;
    }
    .field {
      display: inline-flex;
      flex-direction: column;
      gap: 2px;
      font-size: 12px;
    }
    input,
    select {
      font: inherit;
      padding: 6px 8px;
      border: 1px solid currentColor;
      border-radius: 4px;
      background: transparent;
      color: inherit;
    }
    button {
      font: inherit;
      padding: 6px 12px;
      border: 1px solid currentColor;
      border-radius: 4px;
      background: transparent;
      color: inherit;
      cursor: pointer;
    }
    button:focus-visible,
    input:focus-visible,
    select:focus-visible {
      outline: 2px solid currentColor;
      outline-offset: 2px;
    }
    .panel {
      border: 1px solid currentColor;
      border-radius: 6px;
      padding: 12px;
      margin-bottom: 16px;
    }
    .status {
      font-size: 13px;
      opacity: 0.8;
    }
  `;

  static override properties = {
    settings: { state: true },
    bundle: { state: true },
    loadError: { state: true },
    editing: { state: true },
    statusFilter: { state: true },
  };

  // Reactive state — Lit re-renders when any of these is reassigned
  // via its generated setter. Keep them public so the test harness
  // can introspect; nothing user-facing mutates them.
  public settings: Settings = loadSettings();
  public bundle: Bundle | null = null;
  public loadError = "";
  public editing: string | null = null;
  public statusFilter: "" | TranslationStatus = "";

  private client: Client | undefined;
  private subscription: { close(): void } | undefined;

  /** Test-only seam — replaces the global fetch the SDK uses. */
  public fetchImpl: typeof fetch | undefined;

  public override connectedCallback(): void {
    super.connectedCallback();
    if (this.isConfigured()) void this.boot();
  }

  public override disconnectedCallback(): void {
    super.disconnectedCallback();
    this.subscription?.close();
  }

  private isConfigured(): boolean {
    return Boolean(this.settings.apiUrl && this.settings.apiKey && this.settings.project && this.settings.locale);
  }

  private async boot(): Promise<void> {
    this.subscription?.close();
    this.client = createClient({
      apiUrl: this.settings.apiUrl,
      apiKey: this.settings.apiKey,
      project: this.settings.project,
      ...(this.fetchImpl ? { fetch: this.fetchImpl } : {}),
    });
    await this.refresh();
    this.subscription = this.client.subscribe({
      onEvent: (e) => {
        // SDK already patched its cache; mirror into our local
        // copy so the list re-renders without an extra round-trip.
        if (this.bundle && e.locale === this.settings.locale) {
          this.bundle = {
            ...this.bundle,
            messages: { ...this.bundle.messages, [e.key]: e.value },
            statuses: { ...this.bundle.statuses, [e.key]: e.status },
          };
          this.requestUpdate();
        }
      },
    });
  }

  private async refresh(): Promise<void> {
    if (!this.client) return;
    this.loadError = "";
    try {
      this.bundle = await this.client.bundle(this.settings.locale);
    } catch (err) {
      this.loadError = (err as Error).message;
      this.bundle = null;
    }
  }

  private onSettingsChange(field: keyof Settings, value: string): void {
    this.settings = { ...this.settings, [field]: value };
    saveSettings(this.settings);
    if (this.isConfigured()) void this.boot();
    this.requestUpdate();
  }

  private async onSave(detail: { key: string; value: string; status?: TranslationStatus }): Promise<void> {
    if (!this.settings.apiUrl || !this.settings.apiKey || !this.settings.project) return;
    const url = `${this.settings.apiUrl.replace(/\/+$/, "")}/api/v1/projects/${encodeURIComponent(this.settings.project)}/locales/${encodeURIComponent(this.settings.locale)}/keys/${encodeURIComponent(detail.key)}`;
    const doFetch = this.fetchImpl ?? fetch;
    const res = await doFetch(url, {
      method: "PATCH",
      headers: {
        Authorization: "Bearer " + this.settings.apiKey,
        "Content-Type": "application/json",
      },
      body: JSON.stringify({ value: detail.value, status: detail.status ?? "needs_review" }),
    });
    if (!res.ok) {
      this.loadError = `save: ${res.status} ${res.statusText}`;
      this.requestUpdate();
      return;
    }
    // Optimistic local patch. SSE will arrive shortly and re-apply
    // the canonical value; both paths converge so duplicate work
    // here is harmless.
    if (this.bundle) {
      this.bundle = {
        ...this.bundle,
        messages: { ...this.bundle.messages, [detail.key]: detail.value },
        statuses: { ...this.bundle.statuses, [detail.key]: detail.status ?? "needs_review" },
      };
    }
    this.editing = null;
    this.requestUpdate();
  }

  protected override render() {
    return html`
      <header>
        <h1>Glossa Admin</h1>
        <label class="field">
          <span>API URL</span>
          <input
            type="url"
            .value=${this.settings.apiUrl}
            @change=${(e: Event) => this.onSettingsChange("apiUrl", (e.target as HTMLInputElement).value)}
            placeholder="http://localhost:8080"
            aria-label="API URL"
          />
        </label>
        <label class="field">
          <span>API key</span>
          <input
            type="password"
            .value=${this.settings.apiKey}
            @change=${(e: Event) => this.onSettingsChange("apiKey", (e.target as HTMLInputElement).value)}
            placeholder="glossa_…"
            aria-label="API key"
          />
        </label>
        <label class="field">
          <span>Project</span>
          <input
            type="text"
            .value=${this.settings.project}
            @change=${(e: Event) => this.onSettingsChange("project", (e.target as HTMLInputElement).value)}
            placeholder="my-app"
            aria-label="Project slug"
          />
        </label>
        <label class="field">
          <span>Locale</span>
          <input
            type="text"
            .value=${this.settings.locale}
            @change=${(e: Event) => this.onSettingsChange("locale", (e.target as HTMLInputElement).value)}
            placeholder="de"
            aria-label="Locale"
          />
        </label>
        <label class="field">
          <span>Status filter</span>
          <select
            .value=${this.statusFilter}
            @change=${(e: Event) => {
              this.statusFilter = (e.target as HTMLSelectElement).value as Settings["locale"] as never;
              this.requestUpdate();
            }}
            aria-label="Status filter"
          >
            <option value="">all</option>
            <option value="pending">untranslated / pending</option>
            <option value="needs_review">needs review</option>
            <option value="approved">approved</option>
          </select>
        </label>
      </header>

      ${this.loadError ? html`<div class="panel status" role="alert">${this.loadError}</div>` : null}

      <section class="panel" aria-label="Keys">
        <glossa-admin-key-list
          .messages=${this.bundle?.messages ?? {}}
          .statuses=${this.bundle?.statuses ?? {}}
          .filter=${this.statusFilter}
          .selected=${this.editing}
          @select-key=${(e: CustomEvent<{ key: string }>) => {
            this.editing = e.detail.key;
            this.requestUpdate();
          }}
        ></glossa-admin-key-list>
      </section>

      ${this.editing && this.bundle
        ? html`
            <section class="panel" aria-label="Editor">
              <glossa-admin-key-edit
                .keyName=${this.editing}
                .value=${this.bundle.messages[this.editing] ?? ""}
                .locale=${this.settings.locale}
                @cancel=${() => {
                  this.editing = null;
                  this.requestUpdate();
                }}
                @save=${(e: CustomEvent<{ key: string; value: string; status?: TranslationStatus }>) => void this.onSave(e.detail)}
              ></glossa-admin-key-edit>
            </section>
          `
        : null}

      ${this.bundle && this.isConfigured()
        ? html`
            <section class="panel" aria-label="Live consumer demo">
              <h2 style="font-size:14px;margin:0 0 8px;">Live consumer (dogfoods @glossa/elements)</h2>
              <glossa-admin-demo-strip
                .apiUrl=${this.settings.apiUrl}
                .apiKey=${this.settings.apiKey}
                .project=${this.settings.project}
                .locale=${this.settings.locale}
              ></glossa-admin-demo-strip>
            </section>
          `
        : null}
    `;
  }
}

function storage(): Storage | undefined {
  // Resolve through globalThis so a test-time shim wins over any
  // happy-dom variant that exposes the symbol on window but not on
  // the module's lexical scope.
  const g = globalThis as unknown as { localStorage?: Storage };
  return g.localStorage;
}

function loadSettings(): Settings {
  const s = storage();
  if (!s) return blank();
  const raw = s.getItem(STORAGE_KEY);
  if (!raw) return blank();
  try {
    const parsed = JSON.parse(raw) as Partial<Settings>;
    return {
      apiUrl: parsed.apiUrl ?? "",
      apiKey: parsed.apiKey ?? "",
      project: parsed.project ?? "",
      locale: parsed.locale ?? "",
    };
  } catch {
    return blank();
  }
}

function saveSettings(s: Settings): void {
  const store = storage();
  store?.setItem(STORAGE_KEY, JSON.stringify(s));
}

function blank(): Settings {
  return { apiUrl: "", apiKey: "", project: "", locale: "" };
}

if (!customElements.get("glossa-admin")) {
  customElements.define("glossa-admin", GlossaAdmin);
}

declare global {
  interface HTMLElementTagNameMap {
    "glossa-admin": GlossaAdmin;
  }
}
