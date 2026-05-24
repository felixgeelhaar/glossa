// <gl-tabs current="editor" @gl-tab-change=...>
//   <gl-tab id="editor" label="Editor"></gl-tab>
//   ...
// </gl-tabs>
//
// Pure navigation widget — callers render the tab panel content
// themselves based on `current`. Keeps state outside the component.

import { LitElement, css, html } from "lit";

export class GlTabs extends LitElement {
  static override styles = css`
    :host {
      display: block;
    }
    nav {
      display: flex;
      gap: 2px;
      border-bottom: 1px solid var(--gl-border);
      padding: 0 var(--gl-space-1);
    }
    button {
      font: inherit;
      font-family: var(--gl-font-ui);
      font-size: var(--gl-text-md);
      background: transparent;
      color: var(--gl-text-muted);
      border: none;
      border-bottom: 2px solid transparent;
      padding: 8px 12px;
      cursor: pointer;
      transition:
        color var(--gl-duration-base) var(--gl-ease),
        border-color var(--gl-duration-base) var(--gl-ease);
    }
    button:hover {
      color: var(--gl-text);
    }
    button[aria-current="page"] {
      color: var(--gl-text);
      border-bottom-color: var(--gl-accent);
      font-weight: 500;
    }
    button:focus-visible {
      outline: none;
      box-shadow: inset 0 0 0 2px var(--gl-focus-ring-strong);
      border-radius: var(--gl-radius-sm);
    }
  `;

  static override properties = {
    current: { type: String, reflect: true },
    items: { attribute: false },
  };

  public current = "";
  public items: Array<{ id: string; label: string; hidden?: boolean }> = [];

  private select(id: string): void {
    this.current = id;
    this.dispatchEvent(
      new CustomEvent("gl-tab-change", { detail: { id }, bubbles: true, composed: true }),
    );
  }

  protected override render() {
    const visible = (this.items ?? []).filter((t) => !t.hidden);
    return html`
      <nav aria-label="Sections">
        ${visible.map(
          (t) => html`
            <button
              type="button"
              aria-current=${this.current === t.id ? "page" : "false"}
              @click=${() => this.select(t.id)}
            >
              ${t.label}
            </button>
          `,
        )}
      </nav>
    `;
  }
}

if (!customElements.get("gl-tabs")) {
  customElements.define("gl-tabs", GlTabs);
}

declare global {
  interface HTMLElementTagNameMap {
    "gl-tabs": GlTabs;
  }
}
