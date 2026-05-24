import { LitElement, css, html } from "lit";

export type BadgeVariant = "neutral" | "pending" | "review" | "approved" | "danger" | "accent";

export class GlBadge extends LitElement {
  static override styles = css`
    :host {
      display: inline-block;
    }
    .pill {
      display: inline-flex;
      align-items: center;
      gap: var(--gl-space-1);
      padding: 2px 8px;
      border-radius: var(--gl-radius-pill);
      font-size: var(--gl-text-xs);
      font-weight: 500;
      line-height: 1.5;
      white-space: nowrap;
      border: 1px solid transparent;
    }
    .neutral {
      background: var(--gl-surface-sunken);
      color: var(--gl-text-muted);
      border-color: var(--gl-border);
    }
    .pending {
      background: var(--gl-status-pending-bg);
      color: var(--gl-status-pending);
    }
    .review {
      background: var(--gl-status-review-bg);
      color: var(--gl-status-review);
    }
    .approved {
      background: var(--gl-status-approved-bg);
      color: var(--gl-status-approved);
    }
    .danger {
      background: var(--gl-danger-bg);
      color: var(--gl-danger);
    }
    .accent {
      background: var(--gl-accent-quiet);
      color: var(--gl-accent);
    }
  `;

  static override properties = {
    variant: { type: String },
  };

  public variant: BadgeVariant = "neutral";

  protected override render() {
    return html`<span class=${`pill ${this.variant}`}><slot></slot></span>`;
  }
}

if (!customElements.get("gl-badge")) {
  customElements.define("gl-badge", GlBadge);
}

declare global {
  interface HTMLElementTagNameMap {
    "gl-badge": GlBadge;
  }
}
