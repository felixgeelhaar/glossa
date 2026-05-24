// <gl-theme-toggle> — three-way switch: system / light / dark.
// Persists choice via the theme module; cycles on click.

import { LitElement, css, html } from "lit";

import { getTheme, setTheme, type Theme } from "./theme.js";

export class GlThemeToggle extends LitElement {
  static override styles = css`
    :host { display: inline-block; }
    button {
      font: inherit;
      font-size: var(--gl-text-sm);
      background: transparent;
      color: var(--gl-text-muted);
      border: 1px solid var(--gl-border);
      border-radius: var(--gl-radius-md);
      padding: 4px 10px;
      cursor: pointer;
      display: inline-flex;
      align-items: center;
      gap: var(--gl-space-2);
      height: 28px;
    }
    button:hover { color: var(--gl-text); border-color: var(--gl-border-strong); }
    button:focus-visible {
      outline: 2px solid var(--gl-focus-ring-strong);
      outline-offset: 2px;
    }
    .glyph { font-family: var(--gl-font-mono); font-weight: 600; }
  `;

  static override properties = {
    theme: { state: true },
  };

  public theme: Theme = "system";

  public override connectedCallback(): void {
    super.connectedCallback();
    this.theme = getTheme();
  }

  private cycle(): void {
    const order: Theme[] = ["system", "light", "dark"];
    const next = order[(order.indexOf(this.theme) + 1) % order.length] ?? "system";
    this.theme = next;
    setTheme(next);
    this.requestUpdate();
  }

  protected override render() {
    const glyph = this.theme === "light" ? "☀" : this.theme === "dark" ? "☾" : "◐";
    return html`
      <button
        type="button"
        aria-label="Theme: ${this.theme}"
        title="Theme: ${this.theme} (click to cycle)"
        @click=${() => this.cycle()}
      >
        <span class="glyph">${glyph}</span> ${this.theme}
      </button>
    `;
  }
}

if (!customElements.get("gl-theme-toggle")) {
  customElements.define("gl-theme-toggle", GlThemeToggle);
}

declare global {
  interface HTMLElementTagNameMap {
    "gl-theme-toggle": GlThemeToggle;
  }
}
