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
    .chip .count {
      margin-left: 4px;
      font-variant-numeric: tabular-nums;
      opacity: 0.7;
    }
    .chip[aria-pressed="true"] .count { opacity: 0.85; }
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
    .empty-card code {
      font-family: var(--gl-font-mono);
      font-size: 0.95em;
      padding: 0 4px;
      background: var(--gl-bg);
      border-radius: 3px;
    }
    .pm-tabs { display: flex; gap: 4px; margin: var(--gl-space-2) 0 0; }
    .pm-tab {
      font: inherit;
      font-size: var(--gl-text-sm);
      padding: 4px 10px;
      border-radius: 4px 4px 0 0;
      border: 1px solid var(--gl-border);
      border-bottom: none;
      background: var(--gl-surface);
      color: var(--gl-text-muted);
      cursor: pointer;
    }
    .pm-tab[aria-selected="true"] {
      background: var(--gl-bg);
      color: var(--gl-text);
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
    bulkPhase: { state: true },
    bulkProgress: { state: true },
    bulkTotal: { state: true },
    bulkSecondsLeft: { state: true },
    bulkAborted: { state: true },
    emptyPm: { state: true },
    helpOpen: { state: true },
    initialLocale: { type: String },
    initialStatusFilter: { type: String },
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
  public bulkPhase: "idle" | "countdown" | "running" = "idle";
  public bulkProgress = 0;
  public bulkTotal = 0;
  public bulkSecondsLeft = 0;
  public bulkAborted = false;
  public emptyPm: "npm" | "pnpm" | "bun" | "yarn" = "pnpm";
  public helpOpen = false;
  public initialLocale = "";
  public initialStatusFilter: "" | Status = "";

  private onGlobalKey = (e: KeyboardEvent): void => {
    // Skip when typing in an input/textarea or the edit modal is open.
    const target = e.target as HTMLElement | null;
    if (target) {
      const tag = target.tagName.toLowerCase();
      if (tag === "input" || tag === "textarea" || target.isContentEditable) return;
      // Custom elements that wrap inputs hide the real input in a
      // shadow root; treat any element with role=textbox as input.
      if (target.getAttribute?.("role") === "textbox") return;
    }
    if (this.editing) {
      if (e.key === "Escape") {
        e.preventDefault();
        this.editing = null;
        this.requestUpdate();
      }
      return;
    }
    if (e.key === "?") {
      e.preventDefault();
      this.helpOpen = !this.helpOpen;
      this.requestUpdate();
      return;
    }
    if (e.key === "/") {
      e.preventDefault();
      const search = this.renderRoot.querySelector<HTMLElement>(".search");
      search?.focus();
      return;
    }
    if (!this.bundle) return;
    const keys = Object.keys(this.filteredMessages());
    if (keys.length === 0) return;
    const currentIdx = this.editing ? keys.indexOf(this.editing) : -1;
    if (e.key === "j" || e.key === "ArrowDown") {
      e.preventDefault();
      const next = keys[Math.min(currentIdx + 1, keys.length - 1)] ?? keys[0]!;
      this.editing = next;
      this.requestUpdate();
    } else if (e.key === "k" || e.key === "ArrowUp") {
      e.preventDefault();
      const prev = keys[Math.max(currentIdx - 1, 0)] ?? keys[0]!;
      this.editing = prev;
      this.requestUpdate();
    } else if (e.key === "a" && this.editing) {
      e.preventDefault();
      void this.quickStatus(this.editing, "approved");
    } else if (e.key === "r" && this.editing) {
      e.preventDefault();
      void this.quickStatus(this.editing, "needs_review");
    }
  };

  private async quickStatus(key: string, status: "approved" | "needs_review"): Promise<void> {
    if (!this.bundle) return;
    const value = this.bundle.messages[key] ?? "";
    try {
      await this.client.patchTranslation(this.slug, this.locale, key, { value, status });
      this.bundle = {
        ...this.bundle,
        statuses: { ...this.bundle.statuses, [key]: status },
      };
      this.requestUpdate();
    } catch (e) {
      this.err = (e as Error).message;
    }
  }

  public override connectedCallback(): void {
    super.connectedCallback();
    document.addEventListener("keydown", this.onGlobalKey);
  }

  public override disconnectedCallback(): void {
    super.disconnectedCallback();
    document.removeEventListener("keydown", this.onGlobalKey);
  }

  public override willUpdate(changed: Map<string, unknown>): void {
    if (changed.has("client") || changed.has("slug")) {
      void this.loadLocales();
    }
    if (changed.has("initialLocale") && this.initialLocale && this.initialLocale !== this.locale) {
      this.locale = this.initialLocale;
      void this.loadBundle();
      this.dispatchEvent(new CustomEvent("consumed-initial", { bubbles: true, composed: true }));
    }
    if (changed.has("initialStatusFilter") && this.initialStatusFilter !== undefined) {
      this.statusFilter = this.initialStatusFilter as "" | Status;
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
      const newStatus = (detail.status as Status) ?? "needs_review";
      if (this.bundle) {
        this.bundle = {
          ...this.bundle,
          messages: { ...this.bundle.messages, [detail.key]: detail.value },
          statuses: {
            ...this.bundle.statuses,
            [detail.key]: newStatus,
          },
        };
      }
      // First-approval celebration. Triggers the first time a key on
      // this project ever flips to 'approved'; persisted in
      // localStorage so reloads don't re-fire it.
      if (newStatus === "approved" && typeof localStorage !== "undefined") {
        const flagKey = `glossa-first-approved:${this.slug}`;
        if (!localStorage.getItem(flagKey)) {
          localStorage.setItem(flagKey, new Date().toISOString());
          toast(
            "🎉 First translation approved. Wire <glossa-text key=\"…\"> into your consumer to render it.",
            "ok",
          );
        }
      }
      this.editing = null;
      this.requestUpdate();
    } catch (e) {
      this.err = (e as Error).message;
    }
  }

  // Per-status counts across the *unfiltered* bundle so the chips
  // can advertise how many keys live in each bucket regardless of
  // the active search/filter. Chip counts ignore the search box on
  // purpose — they answer "where is the work" not "where is what I
  // typed".
  private statusCounts(): Record<"" | Status, number> {
    const out: Record<"" | Status, number> = {
      "": 0,
      pending: 0,
      needs_review: 0,
      approved: 0,
      ai_translated: 0,
    };
    if (!this.bundle) return out;
    for (const k of Object.keys(this.bundle.messages)) {
      const status = (this.bundle.statuses[k] ?? "pending") as Status;
      out[""]++;
      if (status in out) out[status]++;
    }
    return out;
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

  // Bulk approve has a 5-second undo window before any API call
  // fires. If the user clicks Undo (or starts a fresh selection) we
  // bail out without side effects. After the window closes we walk
  // the selection serially so audit log + SSE order stays stable;
  // bulkProgress is updated per row so the user sees motion.
  private async bulkApprove(): Promise<void> {
    if (this.selectedKeys.size === 0 || !this.bundle) return;
    const keys = [...this.selectedKeys];

    this.bulkAborted = false;
    this.bulkPending = true;
    this.bulkPhase = "countdown";
    this.bulkProgress = 0;
    this.bulkTotal = keys.length;
    this.requestUpdate();

    // Countdown ticks every second so the user can read it.
    let remaining = 5;
    this.bulkSecondsLeft = remaining;
    await new Promise<void>((resolve) => {
      const id = setInterval(() => {
        if (this.bulkAborted) {
          clearInterval(id);
          resolve();
          return;
        }
        remaining--;
        this.bulkSecondsLeft = remaining;
        this.requestUpdate();
        if (remaining <= 0) {
          clearInterval(id);
          resolve();
        }
      }, 1000);
    });

    if (this.bulkAborted) {
      this.resetBulkState();
      toast("Bulk approve cancelled.", "ok");
      return;
    }

    this.bulkPhase = "running";
    this.requestUpdate();
    let ok = 0;
    let failed = 0;
    for (const key of keys) {
      if (this.bulkAborted) break;
      try {
        const value = this.bundle?.messages[key] ?? "";
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
      this.bulkProgress = ok + failed;
      this.requestUpdate();
    }
    this.resetBulkState();
    toast(`Approved ${ok}${failed ? ` · ${failed} failed` : ""}.`, failed ? "err" : "ok");
  }

  private resetBulkState(): void {
    this.bulkPending = false;
    this.bulkPhase = "idle";
    this.bulkProgress = 0;
    this.bulkTotal = 0;
    this.bulkSecondsLeft = 0;
    this.bulkAborted = false;
    this.selectedKeys = new Set();
    this.requestUpdate();
  }

  private undoBulk(): void {
    this.bulkAborted = true;
    this.requestUpdate();
  }

  private renderHelp() {
    const rows: [string, string][] = [
      ["j / ↓", "Next key"],
      ["k / ↑", "Previous key"],
      ["a", "Approve current key"],
      ["r", "Mark current key needs_review"],
      ["Enter", "Open edit form"],
      ["Esc", "Close edit form / dismiss"],
      ["/", "Focus search"],
      ["?", "Toggle this help"],
    ];
    return html`
      <div class="empty-card" role="dialog" aria-modal="false" aria-label="Keyboard shortcuts">
        <h3>Keyboard shortcuts</h3>
        <table class="gl-table" style="margin-top: var(--gl-space-2);">
          <tbody>
            ${rows.map(
              ([k, v]) => html`<tr><td><code>${k}</code></td><td>${v}</td></tr>`,
            )}
          </tbody>
        </table>
        <p style="margin-top: var(--gl-space-3);">
          Shortcuts skip when an input is focused; press <code>?</code> again to close.
        </p>
      </div>
    `;
  }

  private renderEmptyBundle() {
    const snippets: Record<string, string> = {
      npm: `npm install -D @felixgeelhaar/glossa-cli
npx glossa init    # interactive config
npx glossa scan    # extracts keys + POSTs them here`,
      pnpm: `pnpm add -D @felixgeelhaar/glossa-cli
pnpm exec glossa init
pnpm exec glossa scan`,
      bun: `bun add -d @felixgeelhaar/glossa-cli
bunx glossa init
bunx glossa scan`,
      yarn: `yarn add -D @felixgeelhaar/glossa-cli
yarn glossa init
yarn glossa scan`,
    };
    const pm = this.emptyPm;
    return html`
      <div class="empty-card">
        <h3>No keys yet</h3>
        <p>
          <strong>Run <code>glossa scan</code></strong> in your codebase to extract keys,
          or paste a JSON bundle in <strong>Import / Export</strong>.
        </p>
        <div class="pm-tabs" role="tablist" aria-label="Package manager">
          ${Object.keys(snippets).map(
            (k) => html`<button
              role="tab"
              type="button"
              class="pm-tab"
              aria-selected=${pm === k}
              @click=${() => {
                this.emptyPm = k as "npm" | "pnpm" | "bun" | "yarn";
                this.requestUpdate();
              }}
            >${k}</button>`,
          )}
        </div>
        <pre role="tabpanel">${snippets[pm]}</pre>
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
      ${this.helpOpen ? this.renderHelp() : null}
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
                ${(() => {
                  const counts = this.statusCounts();
                  return STATUS_FILTERS.map(
                    (f) => html`<button
                      class="chip"
                      type="button"
                      aria-pressed=${this.statusFilter === f.value}
                      @click=${() => {
                        this.statusFilter = f.value;
                        this.requestUpdate();
                      }}
                    >${f.label}
                      <span class="count" aria-hidden="true">(${counts[f.value]})</span>
                    </button>`,
                  );
                })()}
              </div>
            </div>
            ${this.bulkPending
              ? html`
                  <div class="selection-bar" role="status" aria-live="polite" aria-busy="true">
                    <span class="grow">
                      ${this.bulkPhase === "countdown"
                        ? html`Approving <strong>${this.bulkTotal}</strong> keys in <strong>${this.bulkSecondsLeft}s</strong>…`
                        : html`Approved <strong>${this.bulkProgress}</strong> / ${this.bulkTotal}…`}
                    </span>
                    <gl-button variant="danger" size="sm" @click=${() => this.undoBulk()}>
                      ${this.bulkPhase === "countdown" ? "Undo" : "Stop"}
                    </gl-button>
                  </div>
                `
              : selCount > 0
              ? html`
                  <div class="selection-bar" aria-live="polite">
                    <span class="grow"><strong>${selCount}</strong> selected</span>
                    <gl-button
                      variant="primary"
                      size="sm"
                      @click=${() => {
                        if (!confirm(`Approve ${selCount} keys?`)) return;
                        void this.bulkApprove();
                      }}
                    >Approve selected</gl-button>
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
