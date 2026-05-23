// Locale CRUD tab — list, add, enable-toggle, delete.

import { LitElement, css, html } from "lit";

import type { adminClient, LocaleRow } from "./api-client.js";

type Client = ReturnType<typeof adminClient>;

export class GlossaAdminLocalesTab extends LitElement {
  static override styles = css`
    :host { display: block; }
    form { display: flex; gap: 8px; align-items: center; margin-bottom: 12px; }
    table { width: 100%; border-collapse: collapse; font-size: 14px; }
    th, td { padding: 6px 8px; border-bottom: 1px solid currentColor; text-align: left; }
    .err { color: #b00020; font-size: 13px; }
  `;

  static override properties = {
    client: { state: true },
    slug: { state: true },
    rows: { state: true },
    err: { state: true },
  };

  public client!: Client;
  public slug = "";
  public rows: LocaleRow[] = [];
  public err = "";

  public override willUpdate(changed: Map<string, unknown>): void {
    if (changed.has("client") || changed.has("slug")) {
      void this.load();
    }
  }

  private async load(): Promise<void> {
    try {
      this.rows = await this.client.listLocales(this.slug);
      this.err = "";
    } catch (e) {
      this.err = (e as Error).message;
    }
  }

  private async onAdd(e: Event): Promise<void> {
    e.preventDefault();
    const form = e.target as HTMLFormElement;
    const fd = new FormData(form);
    const code = String(fd.get("code") ?? "");
    const label = String(fd.get("label") ?? "");
    try {
      await this.client.createLocale(this.slug, { code, label });
      form.reset();
      await this.load();
    } catch (ex) {
      this.err = (ex as Error).message;
    }
  }

  private async onToggle(row: LocaleRow): Promise<void> {
    try {
      await this.client.setLocaleEnabled(this.slug, row.id, !row.enabled);
      await this.load();
    } catch (ex) {
      this.err = (ex as Error).message;
    }
  }

  private async onDelete(row: LocaleRow): Promise<void> {
    if (!confirm(`Delete locale ${row.code}? Translations cascade.`)) return;
    try {
      await this.client.deleteLocale(this.slug, row.id);
      await this.load();
    } catch (ex) {
      this.err = (ex as Error).message;
    }
  }

  protected override render() {
    return html`
      ${this.err ? html`<p class="err" role="alert">${this.err}</p>` : null}
      <form @submit=${(e: Event) => void this.onAdd(e)}>
        <label>Code <input name="code" required placeholder="de-DE" /></label>
        <label>Label <input name="label" required placeholder="Deutsch (DE)" /></label>
        <button type="submit">Add</button>
      </form>
      <table role="grid">
        <thead><tr><th>Code</th><th>Label</th><th>Enabled</th><th>Actions</th></tr></thead>
        <tbody>
          ${this.rows.map(
            (r) => html`
              <tr>
                <td>${r.code}</td>
                <td>${r.label}</td>
                <td>${r.enabled ? "yes" : "no"}</td>
                <td>
                  <button type="button" @click=${() => void this.onToggle(r)}>
                    ${r.enabled ? "Disable" : "Enable"}
                  </button>
                  <button type="button" @click=${() => void this.onDelete(r)}>Delete</button>
                </td>
              </tr>
            `,
          )}
        </tbody>
      </table>
    `;
  }
}

if (!customElements.get("glossa-admin-locales-tab")) {
  customElements.define("glossa-admin-locales-tab", GlossaAdminLocalesTab);
}

declare global {
  interface HTMLElementTagNameMap {
    "glossa-admin-locales-tab": GlossaAdminLocalesTab;
  }
}
