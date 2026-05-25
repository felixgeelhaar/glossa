// Audit log tab — paginated rows from /admin/audit + client-side
// filters. We fetch up to 500 rows once and filter in-memory; the
// data model is small enough that server-side filter params would
// be premature optimization for v1.

import { LitElement, css, html, unsafeCSS } from "lit";

import { glTableStyles } from "@felixgeelhaar/glossa-ui";

import type { adminClient, AuditRow } from "./api-client.js";

type Client = ReturnType<typeof adminClient>;

const ACTOR_FILTERS: { value: "" | "user" | "ai" | "system"; label: string }[] = [
  { value: "", label: "All" },
  { value: "user", label: "User" },
  { value: "ai", label: "AI" },
  { value: "system", label: "System" },
];

const RANGE_OPTIONS = [
  { value: "1d", label: "Last 24h" },
  { value: "7d", label: "Last 7d" },
  { value: "30d", label: "Last 30d" },
  { value: "all", label: "All time" },
];

export class GlossaAdminAuditTab extends LitElement {
  static override styles = css`
    :host {
      display: block;
    }
    ${unsafeCSS(glTableStyles)}
    .err { color: var(--gl-danger); font-size: var(--gl-text-sm); }
    .toolbar {
      display: flex;
      gap: var(--gl-space-2);
      align-items: center;
      flex-wrap: wrap;
      margin-bottom: var(--gl-space-3);
    }
    .search { flex: 1; min-width: 200px; }
    .filters { display: flex; gap: 4px; flex-wrap: wrap; }
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
    .summary {
      color: var(--gl-text-muted);
      font-size: var(--gl-text-sm);
      margin: 0 0 var(--gl-space-2);
    }
  `;

  static override properties = {
    client: { state: true },
    rows: { state: true },
    err: { state: true },
    actorFilter: { state: true },
    range: { state: true },
    search: { state: true },
  };

  public client!: Client;
  public rows: AuditRow[] = [];
  public err = "";
  public actorFilter: "" | "user" | "ai" | "system" = "";
  public range: "1d" | "7d" | "30d" | "all" = "7d";
  public search = "";

  public override willUpdate(changed: Map<string, unknown>): void {
    if (changed.has("client")) void this.load();
  }

  private async load(): Promise<void> {
    try {
      this.rows = await this.client.audit(500);
      this.err = "";
    } catch (e) {
      this.err = (e as Error).message;
    }
  }

  private rangeCutoff(): number {
    if (this.range === "all") return 0;
    const ms = this.range === "1d" ? 86_400_000 : this.range === "7d" ? 604_800_000 : 2_592_000_000;
    return Date.now() - ms;
  }

  private filtered(): AuditRow[] {
    const q = this.search.trim().toLowerCase();
    const cutoff = this.rangeCutoff();
    return this.rows.filter((r) => {
      if (this.actorFilter && (r.actorKind ?? "user") !== this.actorFilter) return false;
      if (cutoff > 0) {
        const t = Date.parse(r.changedAt);
        if (Number.isFinite(t) && t < cutoff) return false;
      }
      if (q) {
        const hay = `${r.beforeValue} ${r.afterValue} ${r.actorLabel ?? ""}`.toLowerCase();
        if (!hay.includes(q)) return false;
      }
      return true;
    });
  }

  protected override render() {
    const rows = this.filtered();
    return html`
      <div class="toolbar">
        <gl-input
          class="search"
          label=""
          placeholder="Search before / after / actor label"
          .value=${this.search}
          @gl-input=${(e: CustomEvent<{ value: string }>) => {
            this.search = e.detail.value;
            this.requestUpdate();
          }}
        ></gl-input>
        <div class="filters" role="group" aria-label="Actor filter">
          ${ACTOR_FILTERS.map(
            (f) => html`<button
              class="chip"
              type="button"
              aria-pressed=${this.actorFilter === f.value}
              @click=${() => {
                this.actorFilter = f.value;
                this.requestUpdate();
              }}
            >${f.label}</button>`,
          )}
        </div>
        <gl-select
          label=""
          .value=${this.range}
          .options=${RANGE_OPTIONS}
          @gl-change=${(e: CustomEvent<{ value: string }>) => {
            this.range = e.detail.value as "1d" | "7d" | "30d" | "all";
            this.requestUpdate();
          }}
        ></gl-select>
      </div>
      ${this.err ? html`<p class="err" role="alert">${this.err}</p>` : null}
      <p class="summary" aria-live="polite">${rows.length} of ${this.rows.length} entries</p>
      ${rows.length === 0
        ? html`<p class="empty">No audit entries match the current filter.</p>`
        : html`
            <table class="gl-table" role="grid">
              <thead>
                <tr>
                  <th>When</th>
                  <th>Translation</th>
                  <th>Before</th>
                  <th>After</th>
                  <th>Actor</th>
                </tr>
              </thead>
              <tbody>
                ${rows.map(
                  (r) => html`
                    <tr>
                      <td class="gl-cell-mono">${formatTime(r.changedAt)}</td>
                      <td class="gl-cell-mono">${(r.translationId ?? "").slice(0, 8)}</td>
                      <td class="value-cell" title=${r.beforeValue}>${r.beforeValue || "—"}</td>
                      <td class="value-cell" title=${r.afterValue}>${r.afterValue}</td>
                      <td>${this.renderActor(r)}</td>
                    </tr>
                  `,
                )}
              </tbody>
            </table>
          `}
    `;
  }

  private renderActor(r: AuditRow) {
    const kind = r.actorKind ?? (r.changedBy ? "user" : "system");
    const label = r.actorLabel || (r.changedBy ? r.changedBy.slice(0, 8) : "system");
    if (kind === "ai") return html`<gl-badge variant="accent">ai: ${label}</gl-badge>`;
    if (kind === "system") return html`<gl-badge variant="neutral">system</gl-badge>`;
    return html`<gl-badge variant="neutral">${label}</gl-badge>`;
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
