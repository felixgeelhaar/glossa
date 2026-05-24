// Bulk import / export tab. Export downloads the current locale's
// bundle as JSON. Import accepts a JSON file with `{ messages }`
// and POSTs to /bulk.

import { LitElement, css, html, unsafeCSS } from "lit";

import { glTableStyles, toast } from "@felixgeelhaar/glossa-ui";

import type { adminClient } from "./api-client.js";

type Client = ReturnType<typeof adminClient>;

export class GlossaAdminBulkTab extends LitElement {
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
    .ok {
      color: var(--gl-success);
      font-size: var(--gl-text-sm);
    }
    .file-input {
      display: inline-flex;
      flex-direction: column;
      gap: var(--gl-space-1);
      font-size: var(--gl-text-sm);
      color: var(--gl-text-muted);
    }
    input[type="file"] {
      font: inherit;
      font-size: var(--gl-text-sm);
      color: var(--gl-text);
    }
  `;

  static override properties = {
    client: { state: true },
    slug: { state: true },
    locale: { state: true },
    locales: { state: true },
    status: { state: true },
    message: { state: true },
    results: { state: true },
  };

  public client!: Client;
  public slug = "";
  public locale = "";
  public locales: { code: string; label: string }[] = [];
  public status = "";
  public message = "";
  public results: Array<{ key: string; id?: string; error?: string }> = [];

  public override willUpdate(changed: Map<string, unknown>): void {
    if (changed.has("client") || changed.has("slug")) {
      void this.loadLocales();
    }
  }

  private async loadLocales(): Promise<void> {
    try {
      const all = await this.client.listLocales(this.slug);
      this.locales = all;
      if (!this.locale && all[0]) this.locale = all[0].code;
    } catch (e) {
      this.message = (e as Error).message;
    }
  }

  private async onExport(): Promise<void> {
    try {
      const bundle = await this.client.listBundle(this.slug, this.locale);
      const json = JSON.stringify(bundle, null, 2);
      const blob = new Blob([json], { type: "application/json" });
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = `${this.slug}-${this.locale}.json`;
      a.click();
      URL.revokeObjectURL(url);
      toast("Exported.", "ok");
    } catch (e) {
      this.message = (e as Error).message;
      this.status = "err";
    }
  }

  private async onImport(file: File): Promise<void> {
    try {
      const text = await file.text();
      const parsed = JSON.parse(text) as { messages?: Record<string, string>; status?: string };
      if (!parsed.messages || typeof parsed.messages !== "object") {
        throw new Error('expected JSON with a "messages" object');
      }
      const out = await this.client.bulkImport(this.slug, this.locale, parsed.messages, parsed.status);
      this.message = `Applied ${out.applied} keys, ${out.failed} failed.`;
      this.status = out.failed === 0 ? "ok" : "err";
      this.results = out.results;
      toast(this.message, this.status === "ok" ? "ok" : "err");
    } catch (e) {
      this.message = (e as Error).message;
      this.status = "err";
      this.results = [];
      toast(this.message, "err");
    }
  }

  protected override render() {
    return html`
      <div class="row">
        <gl-select
          label="Locale"
          .value=${this.locale}
          .options=${this.locales.map((l) => ({ value: l.code, label: `${l.label} (${l.code})` }))}
          @gl-change=${(e: CustomEvent<{ value: string }>) => {
            this.locale = e.detail.value;
          }}
        ></gl-select>
        <gl-button variant="primary" @click=${() => void this.onExport()}>Export JSON</gl-button>
        <label class="file-input">
          <span>Import JSON</span>
          <input
            type="file"
            accept="application/json"
            @change=${(e: Event) => {
              const f = (e.target as HTMLInputElement).files?.[0];
              if (f) void this.onImport(f);
            }}
          />
        </label>
      </div>
      ${this.message
        ? html`<p class=${this.status === "err" ? "err" : "ok"} role="status">${this.message}</p>`
        : null}
      ${this.results.length > 0
        ? html`
            <table class="gl-table" role="grid">
              <thead>
                <tr><th>Key</th><th>Result</th></tr>
              </thead>
              <tbody>
                ${this.results.map(
                  (r) => html`
                    <tr>
                      <td class="gl-cell-mono">${r.key}</td>
                      <td>
                        ${r.error
                          ? html`<gl-badge variant="danger">${r.error}</gl-badge>`
                          : html`<gl-badge variant="approved">applied</gl-badge>`}
                      </td>
                    </tr>
                  `,
                )}
              </tbody>
            </table>
          `
        : null}
    `;
  }
}

if (!customElements.get("glossa-admin-bulk-tab")) {
  customElements.define("glossa-admin-bulk-tab", GlossaAdminBulkTab);
}

declare global {
  interface HTMLElementTagNameMap {
    "glossa-admin-bulk-tab": GlossaAdminBulkTab;
  }
}
