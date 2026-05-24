// User mgmt tab — list, create, update locale scope, delete.

import { LitElement, css, html, unsafeCSS } from "lit";

import { glTableStyles, toast } from "@glossa/ui";

import type { adminClient, UserRow } from "./api-client.js";

type Client = ReturnType<typeof adminClient>;

export class GlossaAdminUsersTab extends LitElement {
  static override styles = css`
    :host {
      display: block;
    }
    ${unsafeCSS(glTableStyles)}
    .row {
      display: grid;
      grid-template-columns: repeat(auto-fit, minmax(160px, 1fr));
      gap: var(--gl-space-3);
      align-items: end;
      margin-bottom: var(--gl-space-3);
    }
    .err {
      color: var(--gl-danger);
      font-size: var(--gl-text-sm);
    }
    .actions {
      display: flex;
      gap: var(--gl-space-2);
    }
  `;

  static override properties = {
    client: { state: true },
    rows: { state: true },
    err: { state: true },
    fEmail: { state: true },
    fPassword: { state: true },
    fRole: { state: true },
    fLocales: { state: true },
  };

  public client!: Client;
  public rows: UserRow[] = [];
  public err = "";
  public fEmail = "";
  public fPassword = "";
  public fRole = "translator";
  public fLocales = "";

  public override willUpdate(changed: Map<string, unknown>): void {
    if (changed.has("client")) void this.load();
  }

  private async load(): Promise<void> {
    try {
      this.rows = await this.client.listUsers();
      this.err = "";
    } catch (e) {
      this.err = (e as Error).message;
    }
  }

  private async onCreate(e: Event): Promise<void> {
    e.preventDefault();
    const locales = this.fLocales
      .split(",")
      .map((s) => s.trim())
      .filter(Boolean);
    try {
      await this.client.createUser({
        email: this.fEmail,
        password: this.fPassword,
        role: this.fRole,
        locales,
      });
      this.fEmail = "";
      this.fPassword = "";
      this.fLocales = "";
      this.fRole = "translator";
      await this.load();
      toast("User created.", "ok");
    } catch (ex) {
      this.err = (ex as Error).message;
      toast(this.err, "err");
    }
  }

  private async onScope(u: UserRow): Promise<void> {
    const input = prompt("Comma-separated locales for " + u.email, u.locales.join(","));
    if (input === null) return;
    const locales = input
      .split(",")
      .map((s) => s.trim())
      .filter(Boolean);
    try {
      await this.client.updateUserLocales(u.id, locales);
      await this.load();
      toast("Scope updated.", "ok");
    } catch (ex) {
      this.err = (ex as Error).message;
      toast(this.err, "err");
    }
  }

  private async onDelete(u: UserRow): Promise<void> {
    if (!confirm(`Delete user ${u.email}?`)) return;
    try {
      await this.client.deleteUser(u.id);
      await this.load();
      toast("User deleted.", "ok");
    } catch (ex) {
      this.err = (ex as Error).message;
      toast(this.err, "err");
    }
  }

  protected override render() {
    return html`
      ${this.err ? html`<p class="err" role="alert">${this.err}</p>` : null}
      <form class="row" @submit=${(e: Event) => void this.onCreate(e)}>
        <gl-input
          label="Email"
          type="email"
          required
          .value=${this.fEmail}
          @gl-input=${(e: CustomEvent<{ value: string }>) => {
            this.fEmail = e.detail.value;
          }}
        ></gl-input>
        <gl-input
          label="Password"
          type="password"
          required
          .value=${this.fPassword}
          @gl-input=${(e: CustomEvent<{ value: string }>) => {
            this.fPassword = e.detail.value;
          }}
        ></gl-input>
        <gl-select
          label="Role"
          .value=${this.fRole}
          .options=${[
            { value: "translator", label: "translator" },
            { value: "admin", label: "admin" },
          ]}
          @gl-change=${(e: CustomEvent<{ value: string }>) => {
            this.fRole = e.detail.value;
          }}
        ></gl-select>
        <gl-input
          label="Locales (csv)"
          placeholder="de,pt-BR"
          .value=${this.fLocales}
          @gl-input=${(e: CustomEvent<{ value: string }>) => {
            this.fLocales = e.detail.value;
          }}
        ></gl-input>
        <gl-button variant="primary" type="submit">Create</gl-button>
      </form>
      <table class="gl-table" role="grid">
        <thead>
          <tr><th>Email</th><th>Role</th><th>Locales</th><th>Actions</th></tr>
        </thead>
        <tbody>
          ${this.rows.map(
            (u) => html`
              <tr>
                <td>${u.email}</td>
                <td>
                  ${u.role === "admin"
                    ? html`<gl-badge variant="accent">admin</gl-badge>`
                    : html`<gl-badge>translator</gl-badge>`}
                </td>
                <td class="gl-cell-mono">${u.locales.join(", ") || "(all)"}</td>
                <td>
                  <div class="actions">
                    <gl-button size="sm" @click=${() => void this.onScope(u)}>Scope</gl-button>
                    <gl-button size="sm" variant="danger" @click=${() => void this.onDelete(u)}>
                      Delete
                    </gl-button>
                  </div>
                </td>
              </tr>
            `,
          )}
        </tbody>
      </table>
    `;
  }
}

if (!customElements.get("glossa-admin-users-tab")) {
  customElements.define("glossa-admin-users-tab", GlossaAdminUsersTab);
}

declare global {
  interface HTMLElementTagNameMap {
    "glossa-admin-users-tab": GlossaAdminUsersTab;
  }
}
