// <glossa-admin-key-list> — sortable, filterable list of keys.
// Uses @felixgeelhaar/glossa-ui tokens for styling + <gl-badge> for the status
// pill so the lifecycle states share their color story with the
// rest of the admin.

import { LitElement, css, html, unsafeCSS } from "lit";

import { glTableStyles } from "@felixgeelhaar/glossa-ui";
import type { TranslationStatus } from "@felixgeelhaar/glossa-sdk";

export class GlossaAdminKeyList extends LitElement {
  static override styles = css`
    :host {
      display: block;
    }
    ${unsafeCSS(glTableStyles)}
    .empty {
      padding: var(--gl-space-4);
      color: var(--gl-text-muted);
      text-align: center;
    }
    .key {
      font-family: var(--gl-font-mono);
      font-size: var(--gl-text-sm);
      color: var(--gl-text);
    }
    .value {
      color: var(--gl-text-muted);
      max-width: 480px;
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }
  `;

  static override properties = {
    messages: { attribute: false },
    statuses: { attribute: false },
    filter: { type: String },
    selected: { type: String },
  };

  public messages: Record<string, string> = {};
  public statuses: Record<string, TranslationStatus> = {};
  public filter: "" | TranslationStatus = "";
  public selected: string | null = null;

  private rows(): Array<{ key: string; value: string; status: TranslationStatus }> {
    const out: Array<{ key: string; value: string; status: TranslationStatus }> = [];
    const messages = this.messages ?? {};
    const statuses = this.statuses ?? {};
    for (const key of Object.keys(messages).sort()) {
      const status = statuses[key] ?? "pending";
      if (this.filter && status !== this.filter) continue;
      out.push({ key, value: messages[key] ?? "", status });
    }
    return out;
  }

  private onClick(key: string): void {
    this.dispatchEvent(new CustomEvent("select-key", { detail: { key }, bubbles: true, composed: true }));
  }

  protected override render() {
    const rows = this.rows();
    if (rows.length === 0) {
      return html`<p class="empty">No keys match the current filter.</p>`;
    }
    return html`
      <table class="gl-table" role="grid">
        <thead>
          <tr>
            <th scope="col">Key</th>
            <th scope="col">Value</th>
            <th scope="col">Status</th>
          </tr>
        </thead>
        <tbody>
          ${rows.map(
            (r) => html`
              <tr
                class="gl-row-clickable"
                tabindex="0"
                role="row"
                aria-selected=${r.key === this.selected}
                @click=${() => this.onClick(r.key)}
                @keydown=${(e: KeyboardEvent) => {
                  if (e.key === "Enter" || e.key === " ") {
                    e.preventDefault();
                    this.onClick(r.key);
                  }
                }}
              >
                <td class="key">${r.key}</td>
                <td class="value">${r.value}</td>
                <td>
                  <gl-badge variant=${badgeVariant(r.status)}>${r.status}</gl-badge>
                </td>
              </tr>
            `,
          )}
        </tbody>
      </table>
    `;
  }
}

function badgeVariant(s: TranslationStatus): "pending" | "review" | "approved" {
  if (s === "approved") return "approved";
  if (s === "needs_review") return "review";
  return "pending";
}

if (!customElements.get("glossa-admin-key-list")) {
  customElements.define("glossa-admin-key-list", GlossaAdminKeyList);
}

declare global {
  interface HTMLElementTagNameMap {
    "glossa-admin-key-list": GlossaAdminKeyList;
  }
}
