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

export type Tab = "editor" | "bulk" | "diff" | "locales" | "users" | "ai" | "audit";

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

  protected override render() {
    if (!this.auth) return this.renderLogin();
    return html`
      <gl-toolbar>
        <span slot="title">Glossa</span>
        <span slot="center" class="ident">
          ${this.auth.tenant.name} · ${this.auth.user.email} · ${this.auth.user.role}
        </span>
        <span slot="actions" class="row">
          <gl-select
            label=""
            .value=${this.activeProject?.slug ?? ""}
            .options=${this.projects.map((p) => ({ value: p.slug, label: p.name }))}
            @gl-change=${(e: CustomEvent<{ value: string }>) => this.onProjectChange(e.detail.value)}
          ></gl-select>
          <gl-theme-toggle></gl-theme-toggle>
          <gl-button variant="ghost" size="sm" @click=${() => this.signOut()}>Sign out</gl-button>
        </span>
      </gl-toolbar>

      <div class="page">
        ${this.activeProject ? this.renderTabs() : html`<p>Create a project to begin.</p>`}
      </div>
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
