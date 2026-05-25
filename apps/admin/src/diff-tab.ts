// Diff tab — Pending / Approved kanban per locale.
//
// Maps the four translation statuses onto the two columns
// reviewers actually work:
//   Pending  = pending + ai_translated (action needed)
//   Approved = approved (done)
//
// `needs_review` is surfaced as a small clickable pill on the
// Pending cell when count > 0; it isn't worth a column of its own.
// Empty state for an "all zero" locale collapses to a single
// "Up to date — nothing waiting" cell so the row isn't a wall of
// zero tiles.
//
// Clicking a cell or pill dispatches `open-in-editor` so admin-app
// can switch to the Editor tab with the matching locale + status
// filter pre-set.

import { LitElement, css, html } from "lit";

import type { adminClient, DiffRow } from "./api-client.js";

type Client = ReturnType<typeof adminClient>;
type ColumnKey = "now" | "next" | "later";

interface ColumnTotals {
  now: number;
  next: number;
  later: number;
  total: number;
}

export class GlossaAdminDiffTab extends LitElement {
  static override styles = css`
    :host { display: block; }
    .err { color: var(--gl-danger); font-size: var(--gl-text-sm); }
    .grid { display: grid; gap: var(--gl-space-3); }
    .locale-row {
      display: grid;
      grid-template-columns: minmax(180px, 1fr) repeat(2, minmax(160px, 1fr));
      gap: var(--gl-space-3);
      align-items: stretch;
    }
    .locale-row.head {
      font-size: var(--gl-text-sm);
      color: var(--gl-text-muted);
      letter-spacing: 0.04em;
      text-transform: uppercase;
    }
    .locale-row.empty {
      grid-template-columns: minmax(180px, 1fr) 1fr;
    }
    .meta { padding: var(--gl-space-3); }
    .meta strong { font-size: var(--gl-text-lg); display: block; }
    .meta .code {
      display: inline-block;
      font-family: var(--gl-font-mono);
      font-size: var(--gl-text-xs);
      color: var(--gl-text-muted);
      background: var(--gl-bg);
      border: 1px solid var(--gl-border);
      padding: 1px 6px;
      border-radius: 999px;
      margin-bottom: 4px;
    }
    .cell {
      font: inherit;
      text-align: left;
      cursor: pointer;
      padding: var(--gl-space-3);
      border-radius: var(--gl-radius-md);
      border: 1px solid var(--gl-border);
      background: var(--gl-surface);
      color: var(--gl-text);
      display: flex;
      flex-direction: column;
      gap: 4px;
      transition: border-color var(--gl-duration-base) var(--gl-ease);
      position: relative;
    }
    .cell:hover, .cell:focus-visible {
      border-color: var(--gl-accent);
      outline: none;
    }
    .cell .count {
      font-size: var(--gl-text-xl);
      font-weight: 600;
      color: var(--gl-text);
    }
    .cell.zero .count { color: var(--gl-text-muted); }
    .cell .label { font-size: var(--gl-text-sm); color: var(--gl-text-muted); }
    .cell.now { border-left: 3px solid var(--gl-pending-fg, var(--gl-accent)); }
    .cell.later { border-left: 3px solid var(--gl-approved-fg, var(--gl-success, #16a34a)); }
    .needs-review-pill {
      align-self: flex-start;
      margin-top: 4px;
      font: inherit;
      font-size: var(--gl-text-xs);
      padding: 2px 8px;
      border-radius: 999px;
      border: 1px solid var(--gl-review-fg, var(--gl-warning, #b45309));
      background: transparent;
      color: var(--gl-review-fg, var(--gl-warning, #b45309));
      cursor: pointer;
    }
    .needs-review-pill:hover, .needs-review-pill:focus-visible {
      background: var(--gl-review-fg, var(--gl-warning, #b45309));
      color: var(--gl-on-accent, white);
      outline: none;
    }
    .uptodate {
      padding: var(--gl-space-3);
      border: 1px dashed var(--gl-border);
      border-radius: var(--gl-radius-md);
      color: var(--gl-text-muted);
      font-size: var(--gl-text-sm);
      display: flex;
      align-items: center;
      gap: var(--gl-space-2);
    }
    .uptodate .ok { color: var(--gl-success, #16a34a); font-weight: 600; }
  `;

  static override properties = {
    client: { state: true },
    slug: { state: true },
    rows: { state: true },
    err: { state: true },
  };

  public client!: Client;
  public slug = "";
  public rows: DiffRow[] = [];
  public err = "";

  public override willUpdate(changed: Map<string, unknown>): void {
    if (changed.has("client") || changed.has("slug")) {
      void this.load();
    }
  }

  private async load(): Promise<void> {
    try {
      const res = await this.client.diff(this.slug);
      this.rows = res.locales;
      this.err = "";
    } catch (e) {
      this.err = (e as Error).message;
    }
  }

  private totals(r: DiffRow): ColumnTotals {
    const now = (r.pending ?? 0) + (r.aiTranslated ?? 0);
    const next = r.needsReview ?? 0;
    const later = r.approved ?? 0;
    return { now, next, later, total: r.total };
  }

  private openInEditor(locale: string, col: ColumnKey): void {
    this.dispatchEvent(
      new CustomEvent("open-in-editor", {
        detail: { locale, column: col },
        bubbles: true,
        composed: true,
      }),
    );
  }

  protected override render() {
    return html`
      ${this.err ? html`<p class="err" role="alert">${this.err}</p>` : null}
      <div class="grid" role="table" aria-label="Translation status per locale">
        <div class="locale-row head" role="row">
          <div role="columnheader">Locale</div>
          <div role="columnheader">Pending</div>
          <div role="columnheader">Approved</div>
        </div>
        ${this.rows.map((r) => {
          const t = this.totals(r);
          if (t.now === 0 && t.next === 0 && t.later === 0) {
            return html`
              <div class="locale-row empty" role="row">
                <div class="meta">
                  <span class="code">${r.locale}</span>
                  <strong>${r.label}</strong>
                  <div class="label">${t.total} keys</div>
                </div>
                <div class="uptodate" role="cell">
                  <span class="ok">✓</span>
                  Up to date — no translations waiting.
                </div>
              </div>
            `;
          }
          if (t.now === 0 && t.next === 0) {
            // Only approved — show a friendlier collapsed row that still
            // surfaces the "All approved" milestone without two zero
            // cells fighting for attention.
            return html`
              <div class="locale-row" role="row">
                <div class="meta">
                  <span class="code">${r.locale}</span>
                  <strong>${r.label}</strong>
                  <div class="label">${t.total} keys</div>
                </div>
                <div class="uptodate" role="cell">
                  <span class="ok">✓</span>
                  All caught up.
                </div>
                <button
                  class="cell later"
                  @click=${() => this.openInEditor(r.locale, "later")}
                  aria-label=${`Approved for ${r.locale}: ${t.later} keys`}
                >
                  <span class="count">${t.later}</span>
                  <span class="label">approved</span>
                </button>
              </div>
            `;
          }
          return html`
            <div class="locale-row" role="row">
              <div class="meta">
                <span class="code">${r.locale}</span>
                <strong>${r.label}</strong>
                <div class="label">${t.total} keys</div>
              </div>
              <button
                class="cell now ${t.now === 0 ? "zero" : ""}"
                @click=${() => this.openInEditor(r.locale, "now")}
                aria-label=${`Pending for ${r.locale}: ${t.now} keys`}
              >
                <span class="count">${t.now}</span>
                <span class="label">${this.nowDetail(r)}</span>
                ${t.next > 0
                  ? html`<button
                      class="needs-review-pill"
                      type="button"
                      @click=${(e: Event) => {
                        e.stopPropagation();
                        this.openInEditor(r.locale, "next");
                      }}
                      aria-label=${`Open ${t.next} keys awaiting review for ${r.locale}`}
                    >
                      ${t.next} needs review →
                    </button>`
                  : null}
              </button>
              <button
                class="cell later ${t.later === 0 ? "zero" : ""}"
                @click=${() => this.openInEditor(r.locale, "later")}
                aria-label=${`Approved for ${r.locale}: ${t.later} keys`}
              >
                <span class="count">${t.later}</span>
                <span class="label">approved</span>
              </button>
            </div>
          `;
        })}
      </div>
    `;
  }

  private nowDetail(r: DiffRow): string {
    const p = r.pending ?? 0;
    const ai = r.aiTranslated ?? 0;
    if (ai > 0 && p > 0) return `${p} pending · ${ai} AI`;
    if (ai > 0) return `${ai} AI-translated`;
    return "pending";
  }
}

if (!customElements.get("glossa-admin-diff-tab")) {
  customElements.define("glossa-admin-diff-tab", GlossaAdminDiffTab);
}

declare global {
  interface HTMLElementTagNameMap {
    "glossa-admin-diff-tab": GlossaAdminDiffTab;
  }
}
