// <glossa-admin-key-list> — sortable, filterable list of keys.
// Uses @felixgeelhaar/glossa-ui tokens for styling + <gl-badge> for the status
// pill so the lifecycle states share their color story with the
// rest of the admin.

import { LitElement, css, html, unsafeCSS } from "lit";

import { glTableStyles } from "@felixgeelhaar/glossa-ui";

type Status = "pending" | "needs_review" | "approved" | "ai_translated";

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
    .check-col {
      width: 36px;
      padding-right: 0;
    }
    input[type="checkbox"] {
      accent-color: var(--gl-accent);
      cursor: pointer;
    }
  `;

  static override properties = {
    messages: { attribute: false },
    statuses: { attribute: false },
    filter: { type: String },
    selected: { type: String },
    selectedKeys: { attribute: false },
  };

  public messages: Record<string, string> = {};
  public statuses: Record<string, Status> = {};
  public filter: "" | Status = "";
  public selected: string | null = null;
  public selectedKeys: Set<string> = new Set();

  private rows(): Array<{ key: string; value: string; status: Status }> {
    const out: Array<{ key: string; value: string; status: Status }> = [];
    const messages = this.messages ?? {};
    const statuses = this.statuses ?? {};
    for (const key of Object.keys(messages).sort()) {
      const status = (statuses[key] as Status) ?? "pending";
      if (this.filter && status !== this.filter) continue;
      out.push({ key, value: messages[key] ?? "", status });
    }
    return out;
  }

  private onSelect(key: string): void {
    this.dispatchEvent(new CustomEvent("select-key", { detail: { key }, bubbles: true, composed: true }));
  }

  private onToggle(key: string): void {
    this.dispatchEvent(new CustomEvent("toggle-key", { detail: { key }, bubbles: true, composed: true }));
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
            <th scope="col" class="check-col" aria-label="Select"></th>
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
                role="row"
                aria-selected=${r.key === this.selected}
              >
                <td class="check-col">
                  <input
                    type="checkbox"
                    aria-label=${`Select ${r.key}`}
                    .checked=${this.selectedKeys?.has(r.key) ?? false}
                    @click=${(e: Event) => e.stopPropagation()}
                    @change=${() => this.onToggle(r.key)}
                  />
                </td>
                <td class="key" tabindex="0" @click=${() => this.onSelect(r.key)}
                    @keydown=${(e: KeyboardEvent) => {
                      if (e.key === "Enter" || e.key === " ") {
                        e.preventDefault();
                        this.onSelect(r.key);
                      }
                    }}>
                  ${r.key}
                </td>
                <td class="value" @click=${() => this.onSelect(r.key)}>${r.value}</td>
                <td @click=${() => this.onSelect(r.key)}>
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

function badgeVariant(s: Status): "pending" | "review" | "approved" | "accent" {
  if (s === "approved") return "approved";
  if (s === "needs_review") return "review";
  if (s === "ai_translated") return "accent";
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
