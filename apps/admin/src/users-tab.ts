// User mgmt tab — list, create, update locale scope, delete.
// Locale scope is comma-separated for simplicity; production users
// will typically have 1-3 locales which fits well in a single
// input. Backend enforces that the last admin can't be deleted.

import { LitElement, css, html } from "lit";

import type { adminClient, UserRow } from "./api-client.js";

type Client = ReturnType<typeof adminClient>;

export class GlossaAdminUsersTab extends LitElement {
  static override styles = css`
    :host { display: block; }
    form { display: flex; gap: 8px; align-items: end; flex-wrap: wrap; margin-bottom: 12px; }
    table { width: 100%; border-collapse: collapse; font-size: 14px; }
    th, td { padding: 6px 8px; border-bottom: 1px solid currentColor; text-align: left; }
    .err { color: #b00020; font-size: 13px; }
    label { display: inline-flex; flex-direction: column; font-size: 12px; }
  `;

  static override properties = {
    client: { state: true },
    rows: { state: true },
    err: { state: true },
  };

  public client!: Client;
  public rows: UserRow[] = [];
  public err = "";

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
    const form = e.target as HTMLFormElement;
    const fd = new FormData(form);
    const email = String(fd.get("email") ?? "");
    const password = String(fd.get("password") ?? "");
    const role = String(fd.get("role") ?? "translator");
    const localesRaw = String(fd.get("locales") ?? "");
    const locales = localesRaw
      .split(",")
      .map((s) => s.trim())
      .filter(Boolean);
    try {
      await this.client.createUser({ email, password, role, locales });
      form.reset();
      await this.load();
    } catch (ex) {
      this.err = (ex as Error).message;
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
    } catch (ex) {
      this.err = (ex as Error).message;
    }
  }

  private async onDelete(u: UserRow): Promise<void> {
    if (!confirm(`Delete user ${u.email}?`)) return;
    try {
      await this.client.deleteUser(u.id);
      await this.load();
    } catch (ex) {
      this.err = (ex as Error).message;
    }
  }

  protected override render() {
    return html`
      ${this.err ? html`<p class="err" role="alert">${this.err}</p>` : null}
      <form @submit=${(e: Event) => void this.onCreate(e)}>
        <label>Email <input name="email" type="email" required /></label>
        <label>Password <input name="password" type="password" required /></label>
        <label>
          Role
          <select name="role">
            <option value="translator">translator</option>
            <option value="admin">admin</option>
          </select>
        </label>
        <label>Locales (csv) <input name="locales" placeholder="de,pt-BR" /></label>
        <button type="submit">Create</button>
      </form>
      <table role="grid">
        <thead><tr><th>Email</th><th>Role</th><th>Locales</th><th>Actions</th></tr></thead>
        <tbody>
          ${this.rows.map(
            (u) => html`
              <tr>
                <td>${u.email}</td>
                <td>${u.role}</td>
                <td>${u.locales.join(", ") || "(all)"}</td>
                <td>
                  <button type="button" @click=${() => void this.onScope(u)}>Scope</button>
                  <button type="button" @click=${() => void this.onDelete(u)}>Delete</button>
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
