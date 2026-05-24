// <glossa-admin> — root SPA. Drives the JWT login flow, the
// project switcher, and the per-project tab set. UI built from
// @felixgeelhaar/glossa-ui primitives so theming + dark mode work consistently.

import { LitElement, css, html } from "lit";

import "@felixgeelhaar/glossa-ui";
import type { GlTabs } from "@felixgeelhaar/glossa-ui";

import {
  adminClient,
  discoverTenants,
  login,
  type AuthState,
  type ProjectRow,
  type TenantOption,
} from "./api-client.js";

const STORAGE_AUTH = "glossa-admin-auth-v2";
const STORAGE_API_URL = "glossa-admin-api-url-v2";
const STORAGE_PROJECT = "glossa-admin-project-v2";

export type Tab = "editor" | "bulk" | "diff" | "locales" | "keys" | "users" | "ai" | "audit";

export class GlossaAdmin extends LitElement {
  static override styles = css`
    :host {
      display: block;
      background: var(--gl-bg);
      color: var(--gl-text);
      font-family: var(--gl-font-ui);
      min-height: 100vh;
    }
    .page {
      max-width: 1200px;
      margin: 0 auto;
      padding: var(--gl-space-5);
    }
    .panel {
      margin-top: var(--gl-space-4);
    }
    .err {
      color: var(--gl-danger);
      background: var(--gl-danger-bg);
      border: 1px solid var(--gl-danger);
      border-radius: var(--gl-radius-md);
      padding: 10px 14px;
      font-size: var(--gl-text-md);
      margin: var(--gl-space-3) 0;
    }
    .ident {
      color: var(--gl-text-muted);
      font-size: var(--gl-text-sm);
    }
    .login-card {
      max-width: 420px;
      margin: var(--gl-space-7) auto;
    }
    .login-fields {
      display: flex;
      flex-direction: column;
      gap: var(--gl-space-3);
    }
    .row {
      display: flex;
      gap: var(--gl-space-2);
      align-items: center;
    }
  `;

  static override properties = {
    apiUrl: { state: true },
    auth: { state: true },
    projects: { state: true },
    activeProject: { state: true },
    tab: { state: true },
    loginError: { state: true },
    loginEmail: { state: true },
    loginPassword: { state: true },
    discoveredTenants: { state: true },
    discoverPending: { state: true },
    createOpen: { state: true },
    createSlug: { state: true },
    createName: { state: true },
    createLocale: { state: true },
    createPending: { state: true },
    createError: { state: true },
    revealedApiKey: { state: true },
    revealedFor: { state: true },
  };

  public apiUrl: string = (typeof localStorage !== "undefined" && localStorage.getItem(STORAGE_API_URL)) || "";
  public auth: AuthState | null = loadAuth();
  public projects: ProjectRow[] = [];
  public activeProject: ProjectRow | null = null;
  public tab: Tab = "editor";
  public loginError = "";

  // Two-step login state. The user types email + password, the SPA
  // calls /auth/discover, and either auto-issues a /auth/login on
  // the single match or shows a tenant picker.
  public loginEmail = "";
  public loginPassword = "";
  public discoveredTenants: TenantOption[] | null = null;
  public discoverPending = false;

  // Create-project dialog state.
  public createOpen = false;
  public createSlug = "";
  public createName = "";
  public createLocale = "de";
  public createPending = false;
  public createError = "";

  // One-shot API-key reveal. After a successful create or rotate
  // the server hands back the raw key once — we show it in a
  // dismissible panel with a copy button. Server only persists the
  // hash, so dismiss = forever.
  public revealedApiKey = "";
  public revealedFor = "";

  public fetchImpl: typeof fetch | undefined;

  public override connectedCallback(): void {
    super.connectedCallback();
    if (this.auth) void this.afterLogin();
  }

  private async afterLogin(): Promise<void> {
    if (!this.auth) return;
    const c = adminClient({
      apiUrl: this.apiUrl,
      token: this.auth.token,
      ...(this.fetchImpl ? { fetch: this.fetchImpl } : {}),
    });
    try {
      this.projects = await c.listProjects();
    } catch {
      this.signOut();
      return;
    }
    const savedSlug = typeof localStorage !== "undefined" ? localStorage.getItem(STORAGE_PROJECT) : null;
    const restored = savedSlug ? this.projects.find((p) => p.slug === savedSlug) : undefined;
    this.activeProject = restored ?? this.projects[0] ?? null;
    this.requestUpdate();
  }

  private async onSubmitCredentials(e: Event): Promise<void> {
    e.preventDefault();
    if (!this.loginEmail || !this.loginPassword) {
      this.loginError = "email and password required";
      this.requestUpdate();
      return;
    }
    this.discoverPending = true;
    this.loginError = "";
    this.requestUpdate();
    try {
      const tenants = await discoverTenants(this.apiUrl, this.loginEmail);
      if (tenants.length === 0) {
        this.loginError = "no account with that email";
        this.discoveredTenants = null;
      } else if (tenants.length === 1) {
        // Single tenant: skip the picker.
        await this.completeLogin(tenants[0]!.slug);
      } else {
        // Multiple tenants: render the picker.
        this.discoveredTenants = tenants;
      }
    } catch (err) {
      this.loginError = (err as Error).message || "login failed";
    } finally {
      this.discoverPending = false;
      this.requestUpdate();
    }
  }

  private async completeLogin(tenantSlug: string): Promise<void> {
    try {
      this.auth = await login(this.apiUrl, tenantSlug, this.loginEmail, this.loginPassword);
      localStorage.setItem(STORAGE_AUTH, JSON.stringify(this.auth));
      localStorage.setItem(STORAGE_API_URL, this.apiUrl);
      this.loginEmail = "";
      this.loginPassword = "";
      this.discoveredTenants = null;
      this.loginError = "";
      await this.afterLogin();
    } catch (err) {
      this.loginError = (err as Error).message || "login failed";
      this.requestUpdate();
    }
  }

  private signOut(): void {
    this.auth = null;
    this.projects = [];
    this.activeProject = null;
    try {
      localStorage.removeItem(STORAGE_AUTH);
      localStorage.removeItem(STORAGE_PROJECT);
    } catch {
      /* ignore */
    }
    this.requestUpdate();
  }

  private onProjectChange(slug: string): void {
    const p = this.projects.find((x) => x.slug === slug) ?? null;
    this.activeProject = p;
    if (p) localStorage.setItem(STORAGE_PROJECT, p.slug);
    this.requestUpdate();
  }

  private openCreate(): void {
    this.createOpen = true;
    this.createSlug = "";
    this.createName = "";
    this.createLocale = "de";
    this.createError = "";
    this.requestUpdate();
  }

  private closeCreate(): void {
    this.createOpen = false;
    this.createError = "";
    this.requestUpdate();
  }

  private async onSubmitCreate(e: Event): Promise<void> {
    e.preventDefault();
    if (!this.auth) return;
    const c = adminClient({
      apiUrl: this.apiUrl,
      token: this.auth.token,
      ...(this.fetchImpl ? { fetch: this.fetchImpl } : {}),
    });
    this.createPending = true;
    this.createError = "";
    this.requestUpdate();
    try {
      const out = await c.createProject({
        tenantId: this.auth.tenant.id,
        slug: this.createSlug.trim(),
        name: this.createName.trim(),
        defaultLocale: this.createLocale.trim() || "de",
      });
      // Refresh the project list, switch to the new project, surface
      // the API key once.
      this.projects = await c.listProjects();
      const created = this.projects.find((p) => p.slug === out.slug) ?? null;
      if (created) {
        this.activeProject = created;
        localStorage.setItem(STORAGE_PROJECT, created.slug);
      }
      this.revealedApiKey = out.apiKey;
      this.revealedFor = `${out.name} (${out.slug})`;
      this.createOpen = false;
    } catch (err) {
      this.createError = (err as Error).message || "create failed";
    } finally {
      this.createPending = false;
      this.requestUpdate();
    }
  }

  private dismissRevealedKey(): void {
    this.revealedApiKey = "";
    this.revealedFor = "";
    this.requestUpdate();
  }

  private async copyRevealedKey(): Promise<void> {
    if (!this.revealedApiKey) return;
    try {
      await navigator.clipboard.writeText(this.revealedApiKey);
    } catch {
      /* clipboard blocked; the input is already select-all-able */
    }
  }

  protected override render() {
    if (!this.auth) return this.renderLogin();
    const isAdmin = this.auth.user.role === "admin";
    return html`
      <gl-toolbar>
        <span slot="title">Glossa</span>
        <span slot="center" class="ident">
          ${this.auth.tenant.name} · ${this.auth.user.email} · ${this.auth.user.role}
        </span>
        <span slot="actions" class="row">
          ${this.projects.length > 0
            ? html`<gl-select
                label=""
                .value=${this.activeProject?.slug ?? ""}
                .options=${this.projects.map((p) => ({ value: p.slug, label: p.name }))}
                @gl-change=${(e: CustomEvent<{ value: string }>) => this.onProjectChange(e.detail.value)}
              ></gl-select>`
            : null}
          ${isAdmin
            ? html`<gl-button variant="outline" size="sm" @click=${() => this.openCreate()}>+ New project</gl-button>`
            : null}
          <gl-theme-toggle></gl-theme-toggle>
          <gl-button variant="ghost" size="sm" @click=${() => this.signOut()}>Sign out</gl-button>
        </span>
      </gl-toolbar>

      <div class="page">
        ${this.revealedApiKey ? this.renderRevealedKey() : null}
        ${this.createOpen ? this.renderCreateProject() : null}
        ${this.activeProject
          ? this.renderTabs()
          : html`<gl-card class="panel">
              <div slot="header">No projects yet</div>
              <div style="padding: var(--gl-space-4);">
                <p style="margin: 0 0 var(--gl-space-3); color: var(--gl-text-muted);">
                  ${isAdmin
                    ? "Create a project to seed translation keys, hand out an API key, and start translating."
                    : "Your admin hasn't created a project yet."}
                </p>
                ${isAdmin
                  ? html`<gl-button variant="primary" @click=${() => this.openCreate()}>Create project</gl-button>`
                  : null}
              </div>
            </gl-card>`}
      </div>
    `;
  }

  private renderCreateProject() {
    return html`
      <gl-card class="panel">
        <div slot="header">New project</div>
        <form
          @submit=${(e: Event) => void this.onSubmitCreate(e)}
          style="display: flex; flex-direction: column; gap: var(--gl-space-3); padding: var(--gl-space-4);"
        >
          <gl-input
            label="Slug"
            required
            placeholder="my-site"
            hint="Lowercase, dotted/dashed identifier. Used in API URLs."
            .value=${this.createSlug}
            @gl-input=${(e: CustomEvent<{ value: string }>) => {
              this.createSlug = e.detail.value;
            }}
          ></gl-input>
          <gl-input
            label="Name"
            required
            placeholder="My site"
            .value=${this.createName}
            @gl-input=${(e: CustomEvent<{ value: string }>) => {
              this.createName = e.detail.value;
            }}
          ></gl-input>
          <gl-input
            label="Default locale"
            placeholder="de"
            hint="BCP-47 subtag. Source-of-truth locale for AI fan-out."
            .value=${this.createLocale}
            @gl-input=${(e: CustomEvent<{ value: string }>) => {
              this.createLocale = e.detail.value;
            }}
          ></gl-input>
          ${this.createError ? html`<div class="err" role="alert">${this.createError}</div>` : null}
          <div class="row">
            <gl-button variant="primary" type="submit" ?disabled=${this.createPending}>
              ${this.createPending ? "Creating…" : "Create"}
            </gl-button>
            <gl-button variant="ghost" type="button" @click=${() => this.closeCreate()}>Cancel</gl-button>
          </div>
        </form>
      </gl-card>
    `;
  }

  private renderRevealedKey() {
    return html`
      <gl-card class="panel">
        <div slot="header">API key for ${this.revealedFor}</div>
        <div style="padding: var(--gl-space-4); display: flex; flex-direction: column; gap: var(--gl-space-3);">
          <p style="margin: 0; color: var(--gl-text-muted);">
            Copy this now. The server only stores its hash — once you dismiss this panel, the raw key is gone forever.
            Anyone with this key can read + write every translation in this project.
          </p>
          <gl-input
            label="Key"
            readonly
            .value=${this.revealedApiKey}
          ></gl-input>
          <div class="row">
            <gl-button variant="primary" @click=${() => void this.copyRevealedKey()}>Copy</gl-button>
            <gl-button variant="ghost" @click=${() => this.dismissRevealedKey()}>I've saved it</gl-button>
          </div>
        </div>
      </gl-card>
    `;
  }

  private renderLogin() {
    // Multi-tenant picker takes over once /discover returns more
    // than one match. Until then, just email + password.
    if (this.discoveredTenants && this.discoveredTenants.length > 1) {
      return this.renderTenantPicker(this.discoveredTenants);
    }
    return html`
      <div class="page">
        <gl-card class="login-card">
          <div slot="header">Sign in to Glossa</div>
          <form @submit=${(e: Event) => void this.onSubmitCredentials(e)} class="login-fields">
            <gl-input
              label="API URL"
              type="url"
              .value=${this.apiUrl}
              hint="Leave empty for same-origin (recommended in production)."
              @gl-input=${(e: CustomEvent<{ value: string }>) => {
                this.apiUrl = e.detail.value;
              }}
            ></gl-input>
            <gl-input
              label="Email"
              type="email"
              required
              autocomplete="username"
              .value=${this.loginEmail}
              @gl-input=${(e: CustomEvent<{ value: string }>) => {
                this.loginEmail = e.detail.value;
              }}
            ></gl-input>
            <gl-input
              label="Password"
              type="password"
              required
              autocomplete="current-password"
              .value=${this.loginPassword}
              @gl-input=${(e: CustomEvent<{ value: string }>) => {
                this.loginPassword = e.detail.value;
              }}
            ></gl-input>
            <gl-button variant="primary" type="submit" ?disabled=${this.discoverPending}>
              ${this.discoverPending ? "Checking…" : "Continue"}
            </gl-button>
            ${this.loginError ? html`<div class="err" role="alert">${this.loginError}</div>` : null}
          </form>
        </gl-card>
      </div>
    `;
  }

  private renderTenantPicker(tenants: TenantOption[]) {
    return html`
      <div class="page">
        <gl-card class="login-card">
          <div slot="header">Pick a workspace</div>
          <div class="login-fields">
            <p style="color: var(--gl-text-muted); font-size: var(--gl-text-md); margin: 0;">
              ${this.loginEmail} is in multiple Glossa tenants. Choose one to continue.
            </p>
            ${tenants.map(
              (t) => html`
                <gl-button
                  variant="outline"
                  @click=${() => void this.completeLogin(t.slug)}
                >
                  ${t.name}
                  <span style="opacity: 0.6; font-family: var(--gl-font-mono); font-size: var(--gl-text-xs);">
                    ${t.slug}
                  </span>
                </gl-button>
              `,
            )}
            <gl-button
              variant="ghost"
              size="sm"
              @click=${() => {
                this.discoveredTenants = null;
                this.loginError = "";
                this.requestUpdate();
              }}
            >
              ← Back
            </gl-button>
            ${this.loginError ? html`<div class="err" role="alert">${this.loginError}</div>` : null}
          </div>
        </gl-card>
      </div>
    `;
  }

  private renderTabs() {
    const isAdmin = this.auth?.user.role === "admin";
    const tabs = [
      { id: "editor", label: "Editor", hidden: false },
      { id: "bulk", label: "Import / Export", hidden: !isAdmin },
      { id: "diff", label: "Diff", hidden: !isAdmin },
      { id: "locales", label: "Locales", hidden: !isAdmin },
      { id: "keys", label: "API keys", hidden: !isAdmin },
      { id: "users", label: "Users", hidden: !isAdmin },
      { id: "ai", label: "AI translation", hidden: !isAdmin },
      { id: "audit", label: "Audit log", hidden: !isAdmin },
    ];
    const slug = this.activeProject!.slug;
    const c = adminClient({
      apiUrl: this.apiUrl,
      token: this.auth!.token,
      ...(this.fetchImpl ? { fetch: this.fetchImpl } : {}),
    });
    return html`
      <gl-tabs
        .current=${this.tab}
        .items=${tabs}
        @gl-tab-change=${(e: CustomEvent<{ id: string }>) => {
          this.tab = e.detail.id as Tab;
          this.requestUpdate();
        }}
      ></gl-tabs>
      <gl-card class="panel" flush>
        <div style="padding: var(--gl-space-4)">
          ${this.tab === "editor"
            ? html`<glossa-admin-editor-tab .client=${c} .slug=${slug} .userRole=${this.auth!.user.role} .scopedLocales=${this.auth!.user.locales}></glossa-admin-editor-tab>`
            : null}
          ${this.tab === "bulk"
            ? html`<glossa-admin-bulk-tab .client=${c} .slug=${slug}></glossa-admin-bulk-tab>`
            : null}
          ${this.tab === "diff"
            ? html`<glossa-admin-diff-tab .client=${c} .slug=${slug}></glossa-admin-diff-tab>`
            : null}
          ${this.tab === "locales"
            ? html`<glossa-admin-locales-tab .client=${c} .slug=${slug}></glossa-admin-locales-tab>`
            : null}
          ${this.tab === "keys"
            ? html`<glossa-admin-keys-tab .client=${c} .slug=${slug}></glossa-admin-keys-tab>`
            : null}
          ${this.tab === "users"
            ? html`<glossa-admin-users-tab .client=${c}></glossa-admin-users-tab>`
            : null}
          ${this.tab === "ai"
            ? html`<glossa-admin-ai-providers-tab .client=${c}></glossa-admin-ai-providers-tab>`
            : null}
          ${this.tab === "audit"
            ? html`<glossa-admin-audit-tab .client=${c}></glossa-admin-audit-tab>`
            : null}
        </div>
      </gl-card>
    `;
  }

  // Keep the GlTabs type import alive at runtime.
  private _refs: { tabs?: GlTabs } = {};
}

function loadAuth(): AuthState | null {
  if (typeof localStorage === "undefined") return null;
  const raw = localStorage.getItem(STORAGE_AUTH);
  if (!raw) return null;
  try {
    const parsed = JSON.parse(raw) as AuthState;
    if (new Date(parsed.expires).getTime() < Date.now()) return null;
    return parsed;
  } catch {
    return null;
  }
}

if (!customElements.get("glossa-admin")) {
  customElements.define("glossa-admin", GlossaAdmin);
}

declare global {
  interface HTMLElementTagNameMap {
    "glossa-admin": GlossaAdmin;
  }
}
