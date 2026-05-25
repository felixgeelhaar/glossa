// <glossa-text key="..."> — flat string lookup. Slot content
// renders when the key is missing or the bundle hasn't loaded
// yet, so the page never shows an empty hole on the first paint.
//
// No wrapper element: the component renders <span style="display:
// contents"> so it inherits the parent's semantics (a <button>
// containing a <glossa-text> still announces as a button).
//
// Hydration UX: while the provider hasn't loaded its first bundle
// yet (ctx.version === 0), the fallback slot is marked with
// `data-glossa-pending` + aria-busy so consumers can style the
// pre-hydration state distinctly. After hydration, a missing key
// surfaces as `data-glossa-missing` instead — useful for strict
// dev builds that want to flag broken keys visually.

import { ContextConsumer } from "@lit/context";
import { LitElement, css, html } from "lit";

import { glossaContext, type GlossaContextValue } from "./context.js";

export class GlossaText extends LitElement {
  static override styles = css`
    :host {
      display: contents;
    }
    :host([data-glossa-pending]) ::slotted(*) {
      opacity: 0.85;
    }
    :host([data-glossa-missing]) ::slotted(*) {
      outline: 1px dotted currentColor;
      outline-offset: 2px;
    }
  `;

  static override properties = {
    key: { type: String },
  };

  public key = "";

  private ctx = new ContextConsumer<typeof glossaContext, this>(this, {
    context: glossaContext,
    subscribe: true,
  });

  protected override render() {
    const ctx = this.ctx.value;
    const value = this.lookup(ctx);
    if (value !== undefined) {
      this.removeAttribute("aria-busy");
      this.removeAttribute("data-glossa-pending");
      this.removeAttribute("data-glossa-missing");
      return html`${value}`;
    }
    const pending = !ctx || ctx.version === 0;
    if (pending) {
      this.setAttribute("aria-busy", "true");
      this.setAttribute("data-glossa-pending", "");
      this.removeAttribute("data-glossa-missing");
    } else {
      this.removeAttribute("aria-busy");
      this.removeAttribute("data-glossa-pending");
      this.setAttribute("data-glossa-missing", "");
    }
    return html`<slot></slot>`;
  }

  private lookup(ctx: GlossaContextValue | undefined): string | undefined {
    if (!ctx) return undefined;
    if (!this.key) return undefined;
    return ctx.get(this.key);
  }
}

if (!customElements.get("glossa-text")) {
  customElements.define("glossa-text", GlossaText);
}

declare global {
  interface HTMLElementTagNameMap {
    "glossa-text": GlossaText;
  }
}
