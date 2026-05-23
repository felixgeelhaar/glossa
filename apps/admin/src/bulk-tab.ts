// Bulk import / export tab.
//
// Export downloads the current locale bundle as a JSON file.
// Import accepts a JSON file with `{ "messages": { ... } }` and
// POSTs it to /bulk; the API ensures keys exist then upserts.

import { LitElement, css, html } from "lit";

import type { adminClient } from "./api-client.js";

type Client = ReturnType<typeof adminClient>;

export class GlossaAdminBulkTab extends LitElement {
  static override styles = css`
    :host { display: block; }
    .row { display: flex; gap: 12px; align-items: center; flex-wrap: wrap; }
    .err { color: #b00020; font-size: 13px; }
    .ok { color: #006600; font-size: 13px; }
    table { width: 100%; border-collapse: collapse; margin-top: 12px; font-size: 13px; }
    th, td { padding: 4px 8px; border-bottom: 1px solid currentColor; text-align: left; }
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
      this.message = "Exported.";
      this.status = "ok";
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
    } catch (e) {
      this.message = (e as Error).message;
      this.status = "err";
      this.results = [];
    }
  }

  protected override render() {
    return html`
      <div class="row">
        <label>
          Locale
          <select
            .value=${this.locale}
            @change=${(e: Event) => {
              this.locale = (e.target as HTMLSelectElement).value;
            }}
            aria-label="Locale"
          >
            ${this.locales.map((l) => html`<option value=${l.code}>${l.label} (${l.code})</option>`)}
          </select>
        </label>
        <button type="button" @click=${() => void this.onExport()}>Export JSON</button>
        <label>
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
            <table role="grid">
              <thead><tr><th>Key</th><th>Result</th></tr></thead>
              <tbody>
                ${this.results.map(
                  (r) =>
                    html`<tr>
                      <td>${r.key}</td>
                      <td>${r.error ? `error: ${r.error}` : r.id}</td>
                    </tr>`,
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
