// <glossa-rich key="..." vars='{"name":"…"}'> — runs the message
// through @glossa/format so ICU plurals / selects / interpolation
// all work. The vars attribute is a JSON string for HTML-ergonomic
// authoring; programmatic callers can also set the property
// directly via `.vars = {...}`.

import { ContextConsumer } from "@lit/context";
import { LitElement, css, html } from "lit";

import { format, type Values } from "@glossa/format";

import { glossaContext, type GlossaContextValue } from "./context.js";

type Vars = Values;

export class GlossaRich extends LitElement {
  static override styles = css`
    :host {
      display: contents;
    }
  `;

  static override properties = {
    key: { type: String },
    vars: { converter: { fromAttribute: (v: string | null) => parseVars(v) } },
  };

  public key = "";
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
      return format(raw, ctx.locale, this.vars);
    } catch (err) {
      if (ctx.strict) {
        // eslint-disable-next-line no-console
        console.warn(`[glossa-rich] format ${this.key}:`, err);
      }
      return raw; // best-effort: surface the raw template
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

if (!customElements.get("glossa-rich")) {
  customElements.define("glossa-rich", GlossaRich);
}

declare global {
  interface HTMLElementTagNameMap {
    "glossa-rich": GlossaRich;
  }
}
