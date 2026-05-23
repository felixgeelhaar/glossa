// Audit log tab — latest 200 mutations, paginated server-side.

import { LitElement, css, html } from "lit";

import type { adminClient, AuditRow } from "./api-client.js";

type Client = ReturnType<typeof adminClient>;

export class GlossaAdminAuditTab extends LitElement {
  static override styles = css`
    :host { display: block; }
    table { width: 100%; border-collapse: collapse; font-size: 13px; }
    th, td { padding: 4px 8px; border-bottom: 1px solid currentColor; text-align: left; vertical-align: top; }
    td { white-space: pre-wrap; word-break: break-word; }
    .err { color: #b00020; font-size: 13px; }
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
    return html`
      ${this.err ? html`<p class="err" role="alert">${this.err}</p>` : null}
      <table role="grid">
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
                <td>${r.changedAt}</td>
                <td>${r.translationId ?? ""}</td>
                <td>${r.beforeValue}</td>
                <td>${r.afterValue}</td>
                <td>${r.changedBy ?? "system"}</td>
              </tr>
            `,
          )}
        </tbody>
      </table>
    `;
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
