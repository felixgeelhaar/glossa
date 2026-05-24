// Diff tab — translation-status snapshot per locale.

import { LitElement, css, html, unsafeCSS } from "lit";

import { glTableStyles } from "@felixgeelhaar/glossa-ui";

import type { adminClient, DiffRow } from "./api-client.js";

type Client = ReturnType<typeof adminClient>;

export class GlossaAdminDiffTab extends LitElement {
  static override styles = css`
    :host {
      display: block;
    }
    ${unsafeCSS(glTableStyles)}
    .err {
      color: var(--gl-danger);
      font-size: var(--gl-text-sm);
    }
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
      <table class="gl-table" role="grid">
        <thead>
          <tr>
            <th scope="col">Locale</th>
            <th scope="col">Label</th>
            <th scope="col" class="gl-cell-num">Total</th>
            <th scope="col" class="gl-cell-num">Pending</th>
            <th scope="col" class="gl-cell-num">Needs review</th>
            <th scope="col" class="gl-cell-num">Approved</th>
          </tr>
        </thead>
        <tbody>
          ${this.rows.map(
            (r) => html`
              <tr>
                <td class="gl-cell-mono">${r.locale}</td>
                <td>${r.label}</td>
                <td class="gl-cell-num">${r.total}</td>
                <td class="gl-cell-num">
                  ${r.pending > 0
                    ? html`<gl-badge variant="pending">${r.pending}</gl-badge>`
                    : html`${r.pending}`}
                </td>
                <td class="gl-cell-num">
                  ${r.needsReview > 0
                    ? html`<gl-badge variant="review">${r.needsReview}</gl-badge>`
                    : html`${r.needsReview}`}
                </td>
                <td class="gl-cell-num">
                  ${r.approved > 0
                    ? html`<gl-badge variant="approved">${r.approved}</gl-badge>`
                    : html`${r.approved}`}
                </td>
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
