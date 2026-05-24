// <gl-table> wraps a native <table> with consistent styling and
// adds a `hoverable` mode for selectable rows. Slot-based — caller
// supplies <thead><tr><th> … <tbody><tr><td> with no per-cell
// classes needed.

import { LitElement, css, html } from "lit";

export class GlTable extends LitElement {
  static override styles = css`
    :host {
      display: block;
      width: 100%;
      overflow-x: auto;
      border: 1px solid var(--gl-border);
      border-radius: var(--gl-radius-lg);
      background: var(--gl-surface);
    }
    /* The slotted table gets styled via ::slotted; we apply the
     * shared rules with :host styling rules that target slotted
     * content via a wrapper. */
    .wrap {
      width: 100%;
    }
    ::slotted(table) {
      width: 100%;
      border-collapse: collapse;
      font-size: var(--gl-text-md);
    }
  `;

  protected override render() {
    return html`<div class="wrap"><slot></slot></div>`;
  }
}

if (!customElements.get("gl-table")) {
  customElements.define("gl-table", GlTable);
}

declare global {
  interface HTMLElementTagNameMap {
    "gl-table": GlTable;
  }
}

/**
 * The slotted table content uses light-DOM CSS. Consumers need to
 * import the matching stylesheet so th/td get borders + padding.
 * Easier than reflecting cells through shadow DOM via Manual slots.
 *
 * Exported as a CSS string so apps can adoptedStyleSheet it:
 *   const sheet = new CSSStyleSheet();
 *   sheet.replaceSync(glTableStyles);
 *   document.adoptedStyleSheets = [...document.adoptedStyleSheets, sheet];
 */
export const glTableStyles = `
.gl-table { width: 100%; border-collapse: collapse; font-size: var(--gl-text-md); }
.gl-table thead th {
  text-align: left;
  font-weight: 600;
  font-size: var(--gl-text-xs);
  text-transform: uppercase;
  letter-spacing: 0.04em;
  color: var(--gl-text-subtle);
  background: var(--gl-surface-sunken);
  padding: 8px 12px;
  border-bottom: 1px solid var(--gl-border);
  white-space: nowrap;
}
.gl-table tbody td {
  padding: 8px 12px;
  border-bottom: 1px solid var(--gl-border);
  color: var(--gl-text);
  vertical-align: middle;
}
.gl-table tbody tr:last-child td { border-bottom: none; }
.gl-table tbody tr.gl-row-clickable { cursor: pointer; }
.gl-table tbody tr.gl-row-clickable:hover { background: var(--gl-surface-sunken); }
.gl-table tbody tr[aria-selected="true"] {
  background: var(--gl-accent-quiet);
}
.gl-table .gl-cell-mono { font-family: var(--gl-font-mono); font-size: var(--gl-text-sm); }
.gl-table .gl-cell-num { text-align: right; font-variant-numeric: tabular-nums; }
`;
