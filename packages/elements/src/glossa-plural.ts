// <glossa-plural key="..." count="3"> — convenience wrapper for
// the most common ICU pattern. Templates look like:
//
//   {count, plural, =0 {none} one {one item} other {# items}}
//
// Internally we just hand `{ count }` to @felixgeelhaar/glossa-format alongside
// any extra `vars` attribute the caller supplies.

import { ContextConsumer } from "@lit/context";
import { LitElement, css, html } from "lit";

import { format, type Values } from "@felixgeelhaar/glossa-format";

import { glossaContext, type GlossaContextValue } from "./context.js";

type Vars = Values;

export class GlossaPlural extends LitElement {
  static override styles = css`
    :host {
      display: contents;
    }
  `;

  static override properties = {
    key: { type: String },
    count: { type: Number },
    vars: { converter: { fromAttribute: (v: string | null) => parseVars(v) } },
  };

  public key = "";
  public count = 0;
  public vars: Vars = {};

  private ctx = new ContextConsumer<typeof glossaContext, this>(this, {
    context: glossaContext,
    subscribe: true,
  });

  protected override render() {
    const value = this.lookup(this.ctx.value);
    return value === undefined ? html`<slot></slot>` : html`${value}`;
  }

  private lookup(ctx: GlossaContextValue | undefined): string | undefined {
    if (!ctx || !this.key) return undefined;
    const raw = ctx.get(this.key);
    if (raw === undefined) return undefined;
    try {
      return format(raw, ctx.locale, { ...this.vars, count: this.count });
    } catch {
      return raw;
    }
  }
}

function parseVars(v: string | null): Vars {
  if (!v) return {};
  try {
    const parsed = JSON.parse(v) as unknown;
    return typeof parsed === "object" && parsed !== null ? (parsed as Vars) : {};
  } catch {
    return {};
  }
}

if (!customElements.get("glossa-plural")) {
  customElements.define("glossa-plural", GlossaPlural);
}

declare global {
  interface HTMLElementTagNameMap {
    "glossa-plural": GlossaPlural;
  }
}
