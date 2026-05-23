// <glossa-admin-key-list> — flat sortable list of keys. Status
// filter is applied before render so the table only ever paints
// what's actually shown.

import { LitElement, css, html } from "lit";

import type { TranslationStatus } from "@glossa/sdk";

export class GlossaAdminKeyList extends LitElement {
  static override styles = css`
    :host {
      display: block;
    }
    table {
      width: 100%;
      border-collapse: collapse;
      font-size: 14px;
    }
    th,
    td {
      padding: 6px 8px;
      text-align: left;
      border-bottom: 1px solid currentColor;
    }
    tbody tr {
      cursor: pointer;
    }
    tbody tr[aria-selected="true"] {
      font-weight: 700;
    }
    .pill {
      display: inline-block;
      padding: 1px 6px;
      border-radius: 999px;
      font-size: 12px;
      border: 1px solid currentColor;
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
    // Defensive defaults — Lit's accessor setup may briefly leave
    // these undefined during the first render with property
    // bindings driven from the parent.
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
      return html`<p>No keys match the current filter.</p>`;
    }
    return html`
      <table role="grid">
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
                <td>${r.key}</td>
                <td>${r.value}</td>
                <td><span class="pill">${r.status}</span></td>
              </tr>
            `,
          )}
        </tbody>
      </table>
    `;
  }
}

if (!customElements.get("glossa-admin-key-list")) {
  customElements.define("glossa-admin-key-list", GlossaAdminKeyList);
}

declare global {
  interface HTMLElementTagNameMap {
    "glossa-admin-key-list": GlossaAdminKeyList;
  }
}
