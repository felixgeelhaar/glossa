// Audit log tab — most-recent mutations, paginated server-side.

import { LitElement, css, html, unsafeCSS } from "lit";

import { glTableStyles } from "@felixgeelhaar/glossa-ui";

import type { adminClient, AuditRow } from "./api-client.js";

type Client = ReturnType<typeof adminClient>;

export class GlossaAdminAuditTab extends LitElement {
  static override styles = css`
    :host {
      display: block;
    }
    ${unsafeCSS(glTableStyles)}
    .err {
      color: var(--gl-danger);
      font-size: var(--gl-text-sm);
    }
    .value-cell {
      max-width: 280px;
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
      font-family: var(--gl-font-mono);
      font-size: var(--gl-text-sm);
    }
    .empty {
      color: var(--gl-text-muted);
      text-align: center;
      padding: var(--gl-space-4);
    }
  `;

  static override properties = {
    client: { state: true },
    rows: { state: true },
    err: { state: true },
  };

  public client!: Client;
  public rows: AuditRow[] = [];
  public err = "";

  public override willUpdate(changed: Map<string, unknown>): void {
    if (changed.has("client")) void this.load();
  }

  private async load(): Promise<void> {
    try {
      this.rows = await this.client.audit(200);
      this.err = "";
    } catch (e) {
      this.err = (e as Error).message;
    }
  }

  protected override render() {
    if (!this.err && this.rows.length === 0) {
      return html`<p class="empty">No audit entries yet.</p>`;
    }
    return html`
      ${this.err ? html`<p class="err" role="alert">${this.err}</p>` : null}
      <table class="gl-table" role="grid">
        <thead>
          <tr>
            <th>When</th>
            <th>Translation</th>
            <th>Before</th>
            <th>After</th>
            <th>By</th>
          </tr>
        </thead>
        <tbody>
          ${this.rows.map(
            (r) => html`
              <tr>
                <td class="gl-cell-mono">${formatTime(r.changedAt)}</td>
                <td class="gl-cell-mono">${(r.translationId ?? "").slice(0, 8)}</td>
                <td class="value-cell" title=${r.beforeValue}>${r.beforeValue || "—"}</td>
                <td class="value-cell" title=${r.afterValue}>${r.afterValue}</td>
                <td>
                  ${r.changedBy
                    ? html`<gl-badge variant="neutral">${r.changedBy.slice(0, 8)}</gl-badge>`
                    : html`<gl-badge variant="neutral">system</gl-badge>`}
                </td>
              </tr>
            `,
          )}
        </tbody>
      </table>
    `;
  }
}

function formatTime(iso: string): string {
  try {
    const d = new Date(iso);
    return d.toLocaleString();
  } catch {
    return iso;
  }
}

if (!customElements.get("glossa-admin-audit-tab")) {
  customElements.define("glossa-admin-audit-tab", GlossaAdminAuditTab);
}

declare global {
  interface HTMLElementTagNameMap {
    "glossa-admin-audit-tab": GlossaAdminAuditTab;
  }
}
