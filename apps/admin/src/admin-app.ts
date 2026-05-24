// <glossa-admin> — root SPA. Drives the JWT login flow, the
// project switcher, and the per-project tab set (editor / bulk
// import / diff / locales / users / audit). One element owns all
// the state; the sub-components are pure-render views.

import { LitElement, css, html } from "lit";

import { adminClient, login, type AuthState, type ProjectRow } from "./api-client.js";

const STORAGE_AUTH = "glossa-admin-auth-v2";
const STORAGE_API_URL = "glossa-admin-api-url-v2";
const STORAGE_PROJECT = "glossa-admin-project-v2";

export type Tab = "editor" | "bulk" | "diff" | "locales" | "users" | "audit";

export class GlossaAdmin extends LitElement {
  static override styles = css`
    :host {
      display: block;
      max-width: 1100px;
      margin: 0 auto;
      padding: 24px;
    }
    header {
      display: flex;
      align-items: center;
      gap: 12px;
      flex-wrap: wrap;
      margin-bottom: 12px;
    }
    nav.tabs {
      display: flex;
      gap: 6px;
      border-bottom: 1px solid currentColor;
      margin-bottom: 16px;
    }
    nav.tabs button {
      background: transparent;
      border: 1px solid currentColor;
      border-bottom: none;
      border-radius: 6px 6px 0 0;
      padding: 6px 10px;
      cursor: pointer;
      font: inherit;
      color: inherit;
    }
    nav.tabs button[aria-current="page"] {
      font-weight: 600;
      background: color-mix(in srgb, currentColor 10%, transparent);
    }
    button,
    select,
    input {
      font: inherit;
      padding: 6px 10px;
      border: 1px solid currentColor;
      border-radius: 4px;
      background: transparent;
      color: inherit;
    }
    .field {
      display: inline-flex;
      flex-direction: column;
      gap: 2px;
      font-size: 12px;
    }
    .panel {
      border: 1px solid currentColor;
      border-radius: 6px;
      padding: 12px;
    }
    h1 {
      margin: 0;
      font-size: 18px;
    }
    .row {
      display: flex;
      gap: 8px;
      align-items: center;
    }
    .err {
      color: #b00020;
      font-size: 13px;
    }
  `;

  static override properties = {
    apiUrl: { state: true },
    auth: { state: true },
    projects: { state: true },
    activeProject: { state: true },
    tab: { state: true },
    loginError: { state: true },
  };

  // Default to same-origin (empty string). The api-client emits
  // relative `/api/v1/...` URLs which the browser resolves against
  // the page origin — works behind the production Traefik ingress
  // and the docker-compose nginx proxy without CORS plumbing.
  public apiUrl: string = (typeof localStorage !== "undefined" && localStorage.getItem(STORAGE_API_URL)) || "";
  public auth: AuthState | null = loadAuth();
  public projects: ProjectRow[] = [];
  public activeProject: ProjectRow | null = null;
  public tab: Tab = "editor";
  public loginError = "";

  /** Test seam — swaps the fetch global the api-client uses. */
  public fetchImpl: typeof fetch | undefined;

  public override connectedCallback(): void {
    super.connectedCallback();
    if (this.auth) void this.afterLogin();
  }

  private async afterLogin(): Promise<void> {
    if (!this.auth) return;
    const c = adminClient({ apiUrl: this.apiUrl, token: this.auth.token, ...(this.fetchImpl ? { fetch: this.fetchImpl } : {}) });
    try {
      this.projects = await c.listProjects();
    } catch {
      // Token may have expired — kick back to login.
      this.signOut();
      return;
    }
    const savedSlug = typeof localStorage !== "undefined" ? localStorage.getItem(STORAGE_PROJECT) : null;
    const restored = savedSlug ? this.projects.find((p) => p.slug === savedSlug) : undefined;
    this.activeProject = restored ?? this.projects[0] ?? null;
    this.requestUpdate();
  }

  private async onLogin(e: Event): Promise<void> {
    e.preventDefault();
    const form = e.target as HTMLFormElement;
    const fd = new FormData(form);
    const tenantSlug = String(fd.get("tenant") ?? "");
    const email = String(fd.get("email") ?? "");
    const password = String(fd.get("password") ?? "");
    try {
      this.auth = await login(this.apiUrl, tenantSlug, email, password);
      localStorage.setItem(STORAGE_AUTH, JSON.stringify(this.auth));
      localStorage.setItem(STORAGE_API_URL, this.apiUrl);
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
      <header>
        <h1>Glossa Admin</h1>
        <span aria-live="polite">
          ${this.auth.tenant.name} · ${this.auth.user.email} (${this.auth.user.role})
        </span>
        <label class="field">
          <span>Project</span>
          <select
            .value=${this.activeProject?.slug ?? ""}
            @change=${(e: Event) => this.onProjectChange((e.target as HTMLSelectElement).value)}
            aria-label="Project switcher"
          >
            ${this.projects.length === 0
              ? html`<option value="">No projects yet</option>`
              : this.projects.map((p) => html`<option value=${p.slug}>${p.name}</option>`)}
          </select>
        </label>
        <button type="button" @click=${() => this.signOut()}>Sign out</button>
      </header>
      ${this.activeProject ? this.renderTabs() : html`<p>Create a project to begin.</p>`}
    `;
  }

  private renderLogin() {
    return html`
      <header><h1>Glossa Admin — Sign in</h1></header>
      <form class="panel" @submit=${(e: Event) => void this.onLogin(e)}>
        <div class="row">
          <label class="field">
            <span>API URL</span>
            <input
              type="url"
              .value=${this.apiUrl}
              @change=${(e: Event) => {
                this.apiUrl = (e.target as HTMLInputElement).value;
              }}
              aria-label="API URL"
              required
            />
          </label>
          <label class="field">
            <span>Tenant</span>
            <input type="text" name="tenant" required aria-label="Tenant slug" />
          </label>
          <label class="field">
            <span>Email</span>
            <input type="email" name="email" required autocomplete="username" aria-label="Email" />
          </label>
          <label class="field">
            <span>Password</span>
            <input
              type="password"
              name="password"
              required
              autocomplete="current-password"
              aria-label="Password"
            />
          </label>
          <button type="submit">Sign in</button>
        </div>
        ${this.loginError ? html`<p class="err" role="alert">${this.loginError}</p>` : null}
      </form>
    `;
  }

  private renderTabs() {
    const isAdmin = this.auth?.user.role === "admin";
    const tabs: Array<{ id: Tab; label: string; adminOnly: boolean }> = [
      { id: "editor", label: "Editor", adminOnly: false },
      { id: "bulk", label: "Bulk import/export", adminOnly: true },
      { id: "diff", label: "Diff", adminOnly: true },
      { id: "locales", label: "Locales", adminOnly: true },
      { id: "users", label: "Users", adminOnly: true },
      { id: "audit", label: "Audit log", adminOnly: true },
    ];
    const visible = tabs.filter((t) => isAdmin || !t.adminOnly);
    const slug = this.activeProject!.slug;
    const c = adminClient({
      apiUrl: this.apiUrl,
      token: this.auth!.token,
      ...(this.fetchImpl ? { fetch: this.fetchImpl } : {}),
    });
    return html`
      <nav class="tabs" aria-label="Sections">
        ${visible.map(
          (t) => html`
            <button
              type="button"
              aria-current=${this.tab === t.id ? "page" : "false"}
              @click=${() => {
                this.tab = t.id;
                this.requestUpdate();
              }}
            >
              ${t.label}
            </button>
          `,
        )}
      </nav>
      <section class="panel">
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
        ${this.tab === "audit"
          ? html`<glossa-admin-audit-tab .client=${c}></glossa-admin-audit-tab>`
          : null}
      </section>
    `;
  }
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
