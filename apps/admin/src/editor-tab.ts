// Editor tab — locale picker + search/status filters + bulk approve
// + key list + edit form. The list / edit components stay dumb;
// filter + selection state lives here so the toolbar can act on it.

import { LitElement, css, html } from "lit";

import { toast } from "@felixgeelhaar/glossa-ui";

import "./key-edit.js";
import "./key-list.js";

import type { adminClient, BundleResponse } from "./api-client.js";
import type { GlossaAdminKeyList } from "./key-list.js";
import type { GlossaAdminKeyEdit } from "./key-edit.js";

type Client = ReturnType<typeof adminClient>;
type Status = "pending" | "needs_review" | "approved" | "ai_translated";

const STATUS_FILTERS: { value: "" | Status; label: string }[] = [
  { value: "", label: "All" },
  { value: "pending", label: "Pending" },
  { value: "ai_translated", label: "AI translated" },
  { value: "needs_review", label: "Needs review" },
  { value: "approved", label: "Approved" },
];

export class GlossaAdminEditorTab extends LitElement {
  static override styles = css`
    :host { display: block; }
    header {
      display: flex;
      gap: var(--gl-space-3);
      align-items: center;
      margin-bottom: var(--gl-space-3);
      flex-wrap: wrap;
    }
    .toolbar {
      display: flex;
      gap: var(--gl-space-2);
      align-items: center;
      flex-wrap: wrap;
      margin-bottom: var(--gl-space-3);
    }
    .search { flex: 1; min-width: 200px; }
    .filters {
      display: flex;
      gap: 4px;
      flex-wrap: wrap;
    }
    .chip {
      font: inherit;
      font-size: var(--gl-text-sm);
      padding: 4px 10px;
      border-radius: 999px;
      border: 1px solid var(--gl-border);
      background: var(--gl-surface);
      color: var(--gl-text-muted);
      cursor: pointer;
    }
    .chip[aria-pressed="true"] {
      background: var(--gl-accent);
      color: var(--gl-on-accent, white);
      border-color: var(--gl-accent);
    }
    .selection-bar {
      display: flex;
      align-items: center;
      gap: var(--gl-space-3);
      padding: var(--gl-space-2) var(--gl-space-3);
      background: var(--gl-surface-elevated, var(--gl-surface));
      border: 1px solid var(--gl-accent);
      border-radius: var(--gl-radius-md);
      margin-bottom: var(--gl-space-3);
      font-size: var(--gl-text-sm);
    }
    .selection-bar .grow { flex: 1; }
    .err { color: var(--gl-danger); font-size: var(--gl-text-sm); }
    .empty-card {
      padding: var(--gl-space-4);
      border: 1px dashed var(--gl-border);
      border-radius: var(--gl-radius-md);
      color: var(--gl-text-muted);
      margin: var(--gl-space-3) 0;
    }
    .empty-card h3 { margin: 0 0 var(--gl-space-2); color: var(--gl-text); }
    .empty-card pre {
      background: var(--gl-bg);
      padding: var(--gl-space-3);
      border-radius: var(--gl-radius-sm);
      font-family: var(--gl-font-mono);
      font-size: var(--gl-text-sm);
      overflow-x: auto;
      margin: var(--gl-space-2) 0;
    }
  `;

  static override properties = {
    client: { state: true },
    slug: { state: true },
    userRole: { state: true },
    scopedLocales: { state: true },
    locales: { state: true },
    locale: { state: true },
    bundle: { state: true },
    editing: { state: true },
    err: { state: true },
    search: { state: true },
    statusFilter: { state: true },
    selectedKeys: { state: true },
    bulkPending: { state: true },
  };

  public client!: Client;
  public slug = "";
  public userRole: "admin" | "translator" = "admin";
  public scopedLocales: string[] = [];
  public locales: { id: string; code: string; label: string }[] = [];
  public locale = "";
  public bundle: BundleResponse | null = null;
  public editing: string | null = null;
  public err = "";
  public search = "";
  public statusFilter: "" | Status = "";
  public selectedKeys: Set<string> = new Set();
  public bulkPending = false;

  public override willUpdate(changed: Map<string, unknown>): void {
    if (changed.has("client") || changed.has("slug")) {
      void this.loadLocales();
    }
  }

  private async loadLocales(): Promise<void> {
    if (!this.client || !this.slug) return;
    try {
      const all = await this.client.listLocales(this.slug);
      const filtered = this.userRole === "translator" && this.scopedLocales.length > 0
        ? all.filter((l) => this.scopedLocales.includes(l.code))
        : all;
      this.locales = filtered;
      if (!this.locale && filtered[0]) this.locale = filtered[0].code;
      if (this.locale) await this.loadBundle();
    } catch (e) {
      this.err = (e as Error).message;
    }
  }

  private async loadBundle(): Promise<void> {
    if (!this.client || !this.slug || !this.locale) return;
    try {
      this.bundle = await this.client.listBundle(this.slug, this.locale);
      this.selectedKeys = new Set();
      this.err = "";
    } catch (e) {
      this.err = (e as Error).message;
    }
  }

  private async onSave(detail: { key: string; value: string; status?: string }): Promise<void> {
    try {
      await this.client.patchTranslation(this.slug, this.locale, detail.key, {
        value: detail.value,
        status: detail.status,
      });
      if (this.bundle) {
        this.bundle = {
          ...this.bundle,
          messages: { ...this.bundle.messages, [detail.key]: detail.value },
          statuses: {
            ...this.bundle.statuses,
            [detail.key]: (detail.status as Status) ?? "needs_review",
          },
        };
      }
      this.editing = null;
      this.requestUpdate();
    } catch (e) {
      this.err = (e as Error).message;
    }
  }

  // Applies search + status filter on top of the bundle messages.
  // The result is the slice the list shows AND the slice bulk-approve
  // operates against.
  private filteredMessages(): Record<string, string> {
    if (!this.bundle) return {};
    const q = this.search.trim().toLowerCase();
    const out: Record<string, string> = {};
    for (const [k, v] of Object.entries(this.bundle.messages)) {
      const status = this.bundle.statuses[k] ?? "pending";
      if (this.statusFilter && status !== this.statusFilter) continue;
      if (q && !k.toLowerCase().includes(q) && !v.toLowerCase().includes(q)) continue;
      out[k] = v;
    }
    return out;
  }

  private toggleKey(key: string): void {
    const next = new Set(this.selectedKeys);
    if (next.has(key)) next.delete(key);
    else next.add(key);
    this.selectedKeys = next;
    this.requestUpdate();
  }

  private selectAllVisible(): void {
    this.selectedKeys = new Set(Object.keys(this.filteredMessages()));
    this.requestUpdate();
  }

  private clearSelection(): void {
    this.selectedKeys = new Set();
    this.requestUpdate();
  }

  private async bulkApprove(): Promise<void> {
    if (this.selectedKeys.size === 0 || !this.bundle) return;
    const keys = [...this.selectedKeys];
    this.bulkPending = true;
    this.requestUpdate();
    let ok = 0;
    let failed = 0;
    // Serial on purpose — keeps audit log + SSE order stable.
    for (const key of keys) {
      try {
        const value = this.bundle.messages[key] ?? "";
        await this.client.patchTranslation(this.slug, this.locale, key, { value, status: "approved" });
        ok++;
        if (this.bundle) {
          this.bundle = {
            ...this.bundle,
            statuses: { ...this.bundle.statuses, [key]: "approved" },
          };
        }
      } catch {
        failed++;
      }
    }
    this.bulkPending = false;
    this.selectedKeys = new Set();
    this.requestUpdate();
    toast(`Approved ${ok}${failed ? ` · ${failed} failed` : ""}.`, failed ? "err" : "ok");
  }

  private renderEmptyBundle() {
    return html`
      <div class="empty-card">
        <h3>No keys yet</h3>
        <p>
          Add translation keys by scanning your codebase with the Glossa CLI,
          or paste a JSON bundle in <strong>Import / Export</strong>.
        </p>
        <pre>pnpm add -D @felixgeelhaar/glossa-cli
pnpx glossa init       # writes glossa.config.json
pnpx glossa scan       # extracts keys + POSTs them here</pre>
        <p>
          Already have a JSON bundle? Open the <strong>Import / Export</strong> tab and paste it in.
        </p>
      </div>
    `;
  }

  protected override render() {
    const visible = this.filteredMessages();
    const totalKeys = this.bundle ? Object.keys(this.bundle.messages).length : 0;
    const selCount = this.selectedKeys.size;
    return html`
      <header>
        <gl-select
          label="Locale"
          .value=${this.locale}
          .options=${this.locales.map((l) => ({ value: l.code, label: `${l.label} (${l.code})` }))}
          @gl-change=${(e: CustomEvent<{ value: string }>) => {
            this.locale = e.detail.value;
            void this.loadBundle();
          }}
        ></gl-select>
      </header>
      ${this.err ? html`<p class="err" role="alert">${this.err}</p>` : null}
      ${totalKeys === 0 && this.locale && this.bundle ? this.renderEmptyBundle() : null}
      ${totalKeys > 0
        ? html`
            <div class="toolbar">
              <gl-input
                class="search"
                label=""
                placeholder="Search keys or values"
                .value=${this.search}
                @gl-input=${(e: CustomEvent<{ value: string }>) => {
                  this.search = e.detail.value;
                  this.requestUpdate();
                }}
              ></gl-input>
              <div class="filters" role="group" aria-label="Status filter">
                ${STATUS_FILTERS.map(
                  (f) => html`<button
                    class="chip"
                    type="button"
                    aria-pressed=${this.statusFilter === f.value}
                    @click=${() => {
                      this.statusFilter = f.value;
                      this.requestUpdate();
                    }}
                  >${f.label}</button>`,
                )}
              </div>
            </div>
            ${selCount > 0
              ? html`
                  <div class="selection-bar">
                    <span class="grow"><strong>${selCount}</strong> selected</span>
                    <gl-button
                      variant="primary"
                      size="sm"
                      ?disabled=${this.bulkPending}
                      @click=${() => void this.bulkApprove()}
                    >${this.bulkPending ? "Approving…" : "Approve selected"}</gl-button>
                    <gl-button variant="ghost" size="sm" @click=${() => this.clearSelection()}>Clear</gl-button>
                  </div>
                `
              : html`
                  <div class="toolbar">
                    <gl-button variant="ghost" size="sm" @click=${() => this.selectAllVisible()}>
                      Select all visible (${Object.keys(visible).length})
                    </gl-button>
                  </div>
                `}
            <glossa-admin-key-list
              .messages=${visible}
              .statuses=${this.bundle?.statuses ?? {}}
              .selected=${this.editing}
              .selectedKeys=${this.selectedKeys}
              @select-key=${(e: CustomEvent<{ key: string }>) => {
                this.editing = e.detail.key;
                this.requestUpdate();
              }}
              @toggle-key=${(e: CustomEvent<{ key: string }>) => this.toggleKey(e.detail.key)}
            ></glossa-admin-key-list>
          `
        : null}
      ${this.editing && this.bundle
        ? html`
            <glossa-admin-key-edit
              .keyName=${this.editing}
              .value=${this.bundle.messages[this.editing] ?? ""}
              .locale=${this.locale}
              @cancel=${() => {
                this.editing = null;
                this.requestUpdate();
              }}
              @save=${(e: CustomEvent<{ key: string; value: string; status?: string }>) => void this.onSave(e.detail)}
            ></glossa-admin-key-edit>
          `
        : null}
    `;
  }

  private _refs = { _l: undefined as GlossaAdminKeyList | undefined, _e: undefined as GlossaAdminKeyEdit | undefined };
}

if (!customElements.get("glossa-admin-editor-tab")) {
  customElements.define("glossa-admin-editor-tab", GlossaAdminEditorTab);
}

declare global {
  interface HTMLElementTagNameMap {
    "glossa-admin-editor-tab": GlossaAdminEditorTab;
  }
}
