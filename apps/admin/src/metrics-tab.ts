// Metrics tab — consumes /admin/projects/:slug/metrics + /admin/metrics
// to surface activation metrics: time-to-first-key-sync,
// time-to-first-translation-edit, time-to-first-consumer-request.
//
// Anchors against the `project_created` event's firstAt as the
// project's t=0. Projects created before migration 0004 won't have
// the event and render "—" for deltas; that's expected.

import { LitElement, css, html } from "lit";

import type {
  adminClient,
  AnalyticsKind,
  ProjectMetricRow,
} from "./api-client.js";

type Client = ReturnType<typeof adminClient>;

interface Milestone {
  kind: AnalyticsKind;
  label: string;
  hint: string;
}

// Each milestone references the non-first event kind; firstAt is
// derived server-side via MIN(occurred_at), so the same row's
// firstAt is the time the milestone was reached. Total is shown
// separately in the Activity totals grid below.
const FIRST_MILESTONES: Milestone[] = [
  {
    kind: "key_synced",
    label: "First key synced",
    hint: "First time `glossa scan` (or import) sent a key.",
  },
  {
    kind: "translation_edited",
    label: "First translation edited",
    hint: "First time someone saved a value in the editor.",
  },
  {
    kind: "consumer_request",
    label: "First consumer request",
    hint: "First time a downstream app hit GET /messages.",
  },
  {
    kind: "ai_translation",
    label: "First AI translation",
    hint: "First time the AI translator filled a key.",
  },
];

const TOTAL_KINDS: { kind: AnalyticsKind; label: string }[] = [
  { kind: "translation_edited", label: "Translations edited" },
  { kind: "consumer_request", label: "Consumer requests" },
  { kind: "ai_translation", label: "AI translations" },
];

export class GlossaAdminMetricsTab extends LitElement {
  static override styles = css`
    :host { display: block; }
    .err { color: var(--gl-danger); font-size: var(--gl-text-sm); }
    .grid {
      display: grid;
      grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
      gap: var(--gl-space-3);
      margin-bottom: var(--gl-space-4);
    }
    .stat {
      padding: var(--gl-space-3);
      border: 1px solid var(--gl-border);
      border-radius: var(--gl-radius-md);
      background: var(--gl-surface);
    }
    .stat .label {
      font-size: var(--gl-text-sm);
      color: var(--gl-text-muted);
      margin-bottom: 4px;
    }
    .stat .value {
      font-size: var(--gl-text-xl);
      font-weight: 600;
      color: var(--gl-text);
      font-variant-numeric: tabular-nums;
    }
    .stat .value.muted { color: var(--gl-text-muted); font-weight: 400; }
    .stat .hint {
      font-size: var(--gl-text-xs);
      color: var(--gl-text-muted);
      margin-top: 6px;
    }
    .stat time {
      display: block;
      font-size: var(--gl-text-xs);
      color: var(--gl-text-muted);
      margin-top: 4px;
    }
    h3 {
      font-size: var(--gl-text-md);
      color: var(--gl-text-muted);
      letter-spacing: 0.04em;
      text-transform: uppercase;
      margin: var(--gl-space-4) 0 var(--gl-space-2);
    }
    h3:first-child { margin-top: 0; }
    .empty {
      padding: var(--gl-space-4);
      border: 1px dashed var(--gl-border);
      border-radius: var(--gl-radius-md);
      color: var(--gl-text-muted);
      text-align: center;
    }
  `;

  static override properties = {
    client: { state: true },
    slug: { state: true },
    rows: { state: true },
    err: { state: true },
  };

  public client!: Client;
  public slug = "";
  public rows: ProjectMetricRow[] = [];
  public err = "";

  public override willUpdate(changed: Map<string, unknown>): void {
    if (changed.has("client") || changed.has("slug")) {
      void this.load();
    }
  }

  private async load(): Promise<void> {
    if (!this.client || !this.slug) return;
    try {
      const res = await this.client.projectMetrics(this.slug);
      this.rows = res.events;
      this.err = "";
    } catch (e) {
      this.err = (e as Error).message;
    }
  }

  private byKind(kind: AnalyticsKind): ProjectMetricRow | undefined {
    return this.rows.find((r) => r.kind === kind);
  }

  // Formats a millisecond delta as the largest reasonable unit. Returns
  // "—" for non-positive deltas (e.g., when the milestone fired before
  // the project_created event — legacy projects).
  private formatDelta(deltaMs: number): string {
    if (!Number.isFinite(deltaMs) || deltaMs <= 0) return "—";
    const sec = Math.floor(deltaMs / 1000);
    if (sec < 60) return `${sec}s`;
    const min = Math.floor(sec / 60);
    if (min < 60) return `${min}m`;
    const hr = Math.floor(min / 60);
    if (hr < 48) return `${hr}h`;
    const days = Math.floor(hr / 24);
    return `${days}d`;
  }

  private formatTime(iso: string): string {
    try {
      return new Date(iso).toLocaleString();
    } catch {
      return iso;
    }
  }

  protected override render() {
    if (this.err) return html`<p class="err" role="alert">${this.err}</p>`;
    if (this.rows.length === 0) {
      return html`
        <p class="empty">
          No metrics yet for this project.
          Once you run <code>glossa scan</code>, edit a translation, or wire a
          consumer, milestones will land here.
        </p>
      `;
    }

    const created = this.byKind("project_created");
    const createdMs = created ? Date.parse(created.firstAt) : NaN;

    return html`
      <h3>Activation milestones</h3>
      <div class="grid">
        ${FIRST_MILESTONES.map((m) => {
          const row = this.byKind(m.kind);
          const reached = !!row;
          let delta = "—";
          let when = "";
          if (row) {
            const ms = Date.parse(row.firstAt);
            if (Number.isFinite(createdMs) && createdMs > 0) {
              delta = this.formatDelta(ms - createdMs);
            } else {
              delta = "reached";
            }
            when = this.formatTime(row.firstAt);
          }
          return html`
            <div class="stat">
              <div class="label">${m.label}</div>
              <div class="value ${reached ? "" : "muted"}">${reached ? delta : "pending"}</div>
              ${when ? html`<time datetime=${row!.firstAt}>${when}</time>` : null}
              <div class="hint">${m.hint}</div>
            </div>
          `;
        })}
      </div>

      <h3>Activity totals</h3>
      <div class="grid">
        ${TOTAL_KINDS.map((t) => {
          const row = this.byKind(t.kind);
          const total = row?.total ?? 0;
          return html`
            <div class="stat">
              <div class="label">${t.label}</div>
              <div class="value ${total === 0 ? "muted" : ""}">${total}</div>
            </div>
          `;
        })}
      </div>
    `;
  }
}

if (!customElements.get("glossa-admin-metrics-tab")) {
  customElements.define("glossa-admin-metrics-tab", GlossaAdminMetricsTab);
}

declare global {
  interface HTMLElementTagNameMap {
    "glossa-admin-metrics-tab": GlossaAdminMetricsTab;
  }
}
