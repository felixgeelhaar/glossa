// Locale CRUD tab — list, add, enable-toggle, delete.

import { LitElement, css, html, unsafeCSS } from "lit";

import { glTableStyles, toast } from "@felixgeelhaar/glossa-ui";

import type { adminClient, LocaleRow } from "./api-client.js";

type Client = ReturnType<typeof adminClient>;

export class GlossaAdminLocalesTab extends LitElement {
  static override styles = css`
    :host {
      display: block;
    }
    ${unsafeCSS(glTableStyles)}
    .row {
      display: flex;
      gap: var(--gl-space-3);
      align-items: end;
      flex-wrap: wrap;
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
    slug: { state: true },
    rows: { state: true },
    err: { state: true },
    formCode: { state: true },
    formLabel: { state: true },
  };

  public client!: Client;
  public slug = "";
  public rows: LocaleRow[] = [];
  public err = "";
  public formCode = "";
  public formLabel = "";

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
    try {
      await this.client.createLocale(this.slug, { code: this.formCode, label: this.formLabel });
      this.formCode = "";
      this.formLabel = "";
      await this.load();
      toast("Locale added.", "ok");
    } catch (ex) {
      this.err = (ex as Error).message;
      toast(this.err, "err");
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
      toast("Locale deleted.", "ok");
    } catch (ex) {
      this.err = (ex as Error).message;
      toast(this.err, "err");
    }
  }

  protected override render() {
    return html`
      ${this.err ? html`<p class="err" role="alert">${this.err}</p>` : null}
      <form class="row" @submit=${(e: Event) => void this.onAdd(e)}>
        <gl-input
          label="Code"
          required
          placeholder="de-DE"
          .value=${this.formCode}
          @gl-input=${(e: CustomEvent<{ value: string }>) => {
            this.formCode = e.detail.value;
          }}
        ></gl-input>
        <gl-input
          label="Label"
          required
          placeholder="Deutsch (DE)"
          .value=${this.formLabel}
          @gl-input=${(e: CustomEvent<{ value: string }>) => {
            this.formLabel = e.detail.value;
          }}
        ></gl-input>
        <gl-button variant="primary" type="submit">Add</gl-button>
      </form>
      <table class="gl-table" role="grid">
        <thead>
          <tr><th>Code</th><th>Label</th><th>Status</th><th>Actions</th></tr>
        </thead>
        <tbody>
          ${this.rows.map(
            (r) => html`
              <tr>
                <td class="gl-cell-mono">${r.code}</td>
                <td>${r.label}</td>
                <td>
                  ${r.enabled
                    ? html`<gl-badge variant="approved">enabled</gl-badge>`
                    : html`<gl-badge variant="pending">disabled</gl-badge>`}
                </td>
                <td>
                  <div class="actions">
                    <gl-button size="sm" @click=${() => void this.onToggle(r)}>
                      ${r.enabled ? "Disable" : "Enable"}
                    </gl-button>
                    <gl-button size="sm" variant="danger" @click=${() => void this.onDelete(r)}>
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

if (!customElements.get("glossa-admin-locales-tab")) {
  customElements.define("glossa-admin-locales-tab", GlossaAdminLocalesTab);
}

declare global {
  interface HTMLElementTagNameMap {
    "glossa-admin-locales-tab": GlossaAdminLocalesTab;
  }
}
