// <glossa-select key="..." value="female"> — ICU select wrapper.
// Templates look like:
//
//   {gender, select, female {…} male {…} other {…}}
//
// The component hands `{ value }` to @felixgeelhaar/glossa-format. The selector
// name inside the template can be anything — callers either match
// it by passing `vars` explicitly, or use a convention. To keep
// the markup simple we expose a single `value` attribute and
// merge it into vars under both `value` and the format-time
// selector if the user has set one via `name`.

import { ContextConsumer } from "@lit/context";
import { LitElement, css, html } from "lit";

import { format, type Values } from "@felixgeelhaar/glossa-format";

import { glossaContext, type GlossaContextValue } from "./context.js";

type Vars = Values;

export class GlossaSelect extends LitElement {
  static override styles = css`
    :host {
      display: contents;
    }
  `;

  static override properties = {
    key: { type: String },
    value: { type: String },
    name: { type: String },
    vars: { converter: { fromAttribute: (v: string | null) => parseVars(v) } },
  };

  public key = "";
  public value = "";
  /**
   * Selector name inside the ICU template. Defaults to `value`
   * which lines up with the simplest authoring style:
   *   {value, select, …}
   */
  public name = "value";
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
      return format(raw, ctx.locale, { ...this.vars, [this.name]: this.value });
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

if (!customElements.get("glossa-select")) {
  customElements.define("glossa-select", GlossaSelect);
}

declare global {
  interface HTMLElementTagNameMap {
    "glossa-select": GlossaSelect;
  }
}
