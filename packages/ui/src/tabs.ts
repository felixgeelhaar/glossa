// <gl-tabs current="editor" @gl-tab-change=...>
//   .items=${[{id, label}, {id, label, group: "more"}]}
//
// Pure navigation widget — callers render the tab panel content
// themselves based on `current`. Items can opt into `group: "more"`
// to be hidden behind an overflow popover, keeping the primary nav
// short. Anything without a group renders inline as a primary tab.

import { LitElement, css, html } from "lit";

export interface GlTabsItem {
  id: string;
  label: string;
  hidden?: boolean;
  group?: "primary" | "more";
}

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
      position: relative;
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
    .more {
      margin-left: auto;
    }
    .more[aria-expanded="true"] {
      color: var(--gl-text);
    }
    .popover {
      position: absolute;
      right: 0;
      top: 100%;
      margin-top: 4px;
      background: var(--gl-surface);
      border: 1px solid var(--gl-border);
      border-radius: var(--gl-radius-md);
      box-shadow: var(--gl-shadow-md, 0 4px 12px rgba(0, 0, 0, 0.12));
      min-width: 180px;
      padding: 4px;
      display: flex;
      flex-direction: column;
      gap: 2px;
      z-index: 10;
    }
    .popover button {
      text-align: left;
      border-radius: var(--gl-radius-sm);
      border-bottom: none;
      padding: 6px 10px;
    }
    .popover button:hover {
      background: var(--gl-bg);
    }
    .popover button[aria-current="page"] {
      background: var(--gl-bg);
      color: var(--gl-text);
      border-bottom: none;
    }
  `;

  static override properties = {
    current: { type: String, reflect: true },
    items: { attribute: false },
    moreOpen: { state: true },
  };

  public current = "";
  public items: GlTabsItem[] = [];
  public moreOpen = false;

  private onDocClick = (e: Event): void => {
    if (!this.moreOpen) return;
    const path = e.composedPath();
    if (!path.includes(this)) {
      this.moreOpen = false;
      this.requestUpdate();
    }
  };

  private onDocKey = (e: KeyboardEvent): void => {
    if (e.key === "Escape" && this.moreOpen) {
      this.moreOpen = false;
      this.requestUpdate();
    }
  };

  public override connectedCallback(): void {
    super.connectedCallback();
    document.addEventListener("click", this.onDocClick);
    document.addEventListener("keydown", this.onDocKey);
  }

  public override disconnectedCallback(): void {
    super.disconnectedCallback();
    document.removeEventListener("click", this.onDocClick);
    document.removeEventListener("keydown", this.onDocKey);
  }

  private select(id: string): void {
    this.current = id;
    this.moreOpen = false;
    this.dispatchEvent(
      new CustomEvent("gl-tab-change", { detail: { id }, bubbles: true, composed: true }),
    );
  }

  protected override render() {
    const visible = (this.items ?? []).filter((t) => !t.hidden);
    const primary = visible.filter((t) => t.group !== "more");
    const more = visible.filter((t) => t.group === "more");
    const moreContainsCurrent = more.some((t) => t.id === this.current);
    return html`
      <nav aria-label="Sections">
        ${primary.map(
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
        ${more.length > 0
          ? html`
              <button
                type="button"
                class="more"
                aria-haspopup="menu"
                aria-expanded=${this.moreOpen}
                aria-current=${moreContainsCurrent ? "page" : "false"}
                @click=${(e: Event) => {
                  e.stopPropagation();
                  this.moreOpen = !this.moreOpen;
                  this.requestUpdate();
                }}
              >
                More
                <span aria-hidden="true">▾</span>
              </button>
              ${this.moreOpen
                ? html`
                    <div class="popover" role="menu">
                      ${more.map(
                        (t) => html`
                          <button
                            type="button"
                            role="menuitem"
                            aria-current=${this.current === t.id ? "page" : "false"}
                            @click=${() => this.select(t.id)}
                          >
                            ${t.label}
                          </button>
                        `,
                      )}
                    </div>
                  `
                : null}
            `
          : null}
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
