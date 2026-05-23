// Diff tab — translation-status snapshot per locale. One table
// row per locale with pending / needs-review / approved counts.

import { LitElement, css, html } from "lit";

import type { adminClient, DiffRow } from "./api-client.js";

type Client = ReturnType<typeof adminClient>;

export class GlossaAdminDiffTab extends LitElement {
  static override styles = css`
    :host { display: block; }
    table { width: 100%; border-collapse: collapse; font-size: 14px; }
    th, td { padding: 6px 8px; border-bottom: 1px solid currentColor; text-align: left; }
    th { text-align: right; }
    th:first-child { text-align: left; }
    td:not(:first-child):not(:nth-child(2)) { text-align: right; font-variant-numeric: tabular-nums; }
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
  public rows: DiffRow[] = [];
  public err = "";

  public override willUpdate(changed: Map<string, unknown>): void {
    if (changed.has("client") || changed.has("slug")) {
      void this.load();
    }
  }

  private async load(): Promise<void> {
    try {
      const res = await this.client.diff(this.slug);
      this.rows = res.locales;
      this.err = "";
    } catch (e) {
      this.err = (e as Error).message;
    }
  }

  protected override render() {
    return html`
      ${this.err ? html`<p class="err" role="alert">${this.err}</p>` : null}
      <table role="grid">
        <thead>
          <tr>
            <th scope="col">Locale</th>
            <th scope="col">Label</th>
            <th scope="col">Total</th>
            <th scope="col">Pending</th>
            <th scope="col">Needs review</th>
            <th scope="col">Approved</th>
          </tr>
        </thead>
        <tbody>
          ${this.rows.map(
            (r) => html`
              <tr>
                <td>${r.locale}</td>
                <td>${r.label}</td>
                <td>${r.total}</td>
                <td>${r.pending}</td>
                <td>${r.needsReview}</td>
                <td>${r.approved}</td>
              </tr>
            `,
          )}
        </tbody>
      </table>
    `;
  }
}

if (!customElements.get("glossa-admin-diff-tab")) {
  customElements.define("glossa-admin-diff-tab", GlossaAdminDiffTab);
}

declare global {
  interface HTMLElementTagNameMap {
    "glossa-admin-diff-tab": GlossaAdminDiffTab;
  }
}
