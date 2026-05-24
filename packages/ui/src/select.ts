import { LitElement, css, html } from "lit";

export interface GlSelectOption {
  value: string;
  label: string;
}

export class GlSelect extends LitElement {
  static formAssociated = true;

  static override styles = css`
    :host {
      display: block;
    }
    label {
      display: flex;
      flex-direction: column;
      gap: var(--gl-space-1);
      font-size: var(--gl-text-sm);
      color: var(--gl-text-muted);
    }
    select {
      font: inherit;
      font-family: var(--gl-font-ui);
      font-size: var(--gl-text-base);
      color: var(--gl-text);
      background: var(--gl-surface);
      border: 1px solid var(--gl-border);
      border-radius: var(--gl-radius-md);
      padding: 8px 10px;
      height: 36px;
      width: 100%;
      box-sizing: border-box;
      cursor: pointer;
      transition: border-color var(--gl-duration-base) var(--gl-ease);
    }
    select:focus-visible {
      outline: none;
      border-color: var(--gl-focus-ring-strong);
      box-shadow: 0 0 0 3px var(--gl-focus-ring);
    }
  `;

  static override properties = {
    label: { type: String },
    name: { type: String },
    value: { type: String },
    options: { attribute: false },
  };

  public label = "";
  public name = "";
  public value = "";
  public options: GlSelectOption[] = [];

  private internals: ElementInternals | null = null;

  public constructor() {
    super();
    if (typeof this.attachInternals === "function") {
      this.internals = this.attachInternals();
    }
  }

  public override willUpdate(changed: Map<string, unknown>): void {
    if (changed.has("value") && this.internals && typeof this.internals.setFormValue === "function") {
      this.internals.setFormValue(this.value);
    }
  }

  private onChange(e: Event): void {
    const v = (e.target as HTMLSelectElement).value;
    this.value = v;
    this.dispatchEvent(new CustomEvent("gl-change", { detail: { value: v }, bubbles: true, composed: true }));
  }

  protected override render() {
    return html`
      <label>
        ${this.label ? html`<span>${this.label}</span>` : null}
        <select name=${this.name} .value=${this.value} @change=${(e: Event) => this.onChange(e)}>
          ${(this.options ?? []).map(
            (o) => html`<option value=${o.value} ?selected=${o.value === this.value}>${o.label}</option>`,
          )}
        </select>
      </label>
    `;
  }
}

if (!customElements.get("gl-select")) {
  customElements.define("gl-select", GlSelect);
}

declare global {
  interface HTMLElementTagNameMap {
    "gl-select": GlSelect;
  }
}
