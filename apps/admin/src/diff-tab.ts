// Diff tab — Now/Next/Later kanban per locale.
//
// Maps the four translation statuses onto the three columns
// reviewers actually work:
//   Now   = pending + ai_translated (action needed)
//   Next  = needs_review (queued for a translator's eyes)
//   Later = approved (done)
//
// Clicking a column cell dispatches an `open-in-editor` event the
// parent (admin-app) listens to so it can switch to the Editor tab
// with the matching locale + status filter pre-set.

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
      grid-template-columns: minmax(180px, 1fr) repeat(3, minmax(160px, 1fr));
      gap: var(--gl-space-3);
      align-items: stretch;
    }
    .locale-row.head {
      font-size: var(--gl-text-sm);
      color: var(--gl-text-muted);
      letter-spacing: 0.04em;
      text-transform: uppercase;
    }
    .meta { padding: var(--gl-space-3); }
    .meta strong { font-size: var(--gl-text-lg); display: block; }
    .meta .code { font-family: var(--gl-font-mono); color: var(--gl-text-muted); }
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
    .cell.next { border-left: 3px solid var(--gl-review-fg, var(--gl-accent)); }
    .cell.later { border-left: 3px solid var(--gl-approved-fg, var(--gl-success, #16a34a)); }
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
          <div role="columnheader">Now (pending + AI)</div>
          <div role="columnheader">Next (review)</div>
          <div role="columnheader">Later (approved)</div>
        </div>
        ${this.rows.map((r) => {
          const t = this.totals(r);
          return html`
            <div class="locale-row" role="row">
              <div class="meta">
                <strong>${r.label}</strong>
                <span class="code">${r.locale}</span>
                <div class="label">${t.total} keys</div>
              </div>
              <button
                class="cell now ${t.now === 0 ? "zero" : ""}"
                @click=${() => this.openInEditor(r.locale, "now")}
                aria-label=${`Now for ${r.locale}: ${t.now} keys`}
              >
                <span class="count">${t.now}</span>
                <span class="label">${this.nowDetail(r)}</span>
              </button>
              <button
                class="cell next ${t.next === 0 ? "zero" : ""}"
                @click=${() => this.openInEditor(r.locale, "next")}
                aria-label=${`Next for ${r.locale}: ${t.next} keys awaiting review`}
              >
                <span class="count">${t.next}</span>
                <span class="label">awaiting review</span>
              </button>
              <button
                class="cell later ${t.later === 0 ? "zero" : ""}"
                @click=${() => this.openInEditor(r.locale, "later")}
                aria-label=${`Later for ${r.locale}: ${t.later} approved keys`}
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
