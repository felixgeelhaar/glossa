// <gl-toolbar> — top app strip. Hosts the brand, project picker,
// theme toggle, and a sign-out button via slots so consumers
// compose without rewriting the chrome.

import { LitElement, css, html } from "lit";

export class GlToolbar extends LitElement {
  static override styles = css`
    :host {
      display: block;
      background: var(--gl-surface);
      border-bottom: 1px solid var(--gl-border);
      padding: var(--gl-space-3) var(--gl-space-5);
    }
    .row {
      display: flex;
      align-items: center;
      gap: var(--gl-space-4);
      flex-wrap: wrap;
    }
    .brand {
      display: inline-flex;
      align-items: center;
      gap: var(--gl-space-2);
      font-size: var(--gl-text-lg);
      font-weight: 600;
      color: var(--gl-text);
      letter-spacing: -0.01em;
    }
    .brand-mark {
      width: 22px;
      height: 22px;
      border-radius: 6px;
      background: var(--gl-accent);
      display: inline-block;
      position: relative;
    }
    .brand-mark::after {
      content: "g";
      position: absolute;
      inset: 0;
      display: flex;
      align-items: center;
      justify-content: center;
      color: var(--gl-accent-fg);
      font-family: var(--gl-font-mono);
      font-weight: 700;
      font-size: 14px;
      line-height: 1;
    }
    .spacer {
      flex: 1;
    }
  `;

  protected override render() {
    return html`
      <div class="row">
        <span class="brand">
          <span class="brand-mark" aria-hidden="true"></span>
          <slot name="title">Glossa</slot>
        </span>
        <slot name="center"></slot>
        <span class="spacer"></span>
        <slot name="actions"></slot>
      </div>
    `;
  }
}

if (!customElements.get("gl-toolbar")) {
  customElements.define("gl-toolbar", GlToolbar);
}

declare global {
  interface HTMLElementTagNameMap {
    "gl-toolbar": GlToolbar;
  }
}
