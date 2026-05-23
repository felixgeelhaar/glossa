// <glossa-text key="..."> — flat string lookup. Slot content
// renders when the key is missing or the bundle hasn't loaded
// yet, so the page never shows an empty hole on the first paint.
//
// No wrapper element: the component renders <span style="display:
// contents"> so it inherits the parent's semantics (a <button>
// containing a <glossa-text> still announces as a button).

import { ContextConsumer } from "@lit/context";
import { LitElement, css, html } from "lit";

import { glossaContext, type GlossaContextValue } from "./context.js";

export class GlossaText extends LitElement {
  static override styles = css`
    :host {
      display: contents;
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
    const value = this.lookup(this.ctx.value);
    return value === undefined ? html`<slot></slot>` : html`${value}`;
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
