import { LitElement, css, html } from "lit";

export class GlCard extends LitElement {
  static override styles = css`
    :host {
      display: block;
      background: var(--gl-surface);
      border: 1px solid var(--gl-border);
      border-radius: var(--gl-radius-lg);
      overflow: hidden;
    }
    ::slotted([slot="header"]) {
      padding: var(--gl-space-3) var(--gl-space-4);
      border-bottom: 1px solid var(--gl-border);
      font-weight: 600;
      font-size: var(--gl-text-md);
      color: var(--gl-text);
      background: var(--gl-surface-sunken);
    }
    .body {
      padding: var(--gl-space-4);
    }
    .body.flush {
      padding: 0;
    }
  `;

  static override properties = {
    flush: { type: Boolean },
  };

  public flush = false;

  protected override render() {
    return html`
      <slot name="header"></slot>
      <div class=${this.flush ? "body flush" : "body"}>
        <slot></slot>
      </div>
    `;
  }
}

if (!customElements.get("gl-card")) {
  customElements.define("gl-card", GlCard);
}

declare global {
  interface HTMLElementTagNameMap {
    "gl-card": GlCard;
  }
}
