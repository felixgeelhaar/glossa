import { LitElement, css, html } from "lit";

export class GlTextarea extends LitElement {
  static formAssociated = true;

  static override styles = css`
    :host { display: block; }
    label {
      display: flex;
      flex-direction: column;
      gap: var(--gl-space-1);
      font-size: var(--gl-text-sm);
      color: var(--gl-text-muted);
    }
    textarea {
      font-family: var(--gl-font-mono);
      font-size: var(--gl-text-base);
      color: var(--gl-text);
      background: var(--gl-surface);
      border: 1px solid var(--gl-border);
      border-radius: var(--gl-radius-md);
      padding: 8px 10px;
      width: 100%;
      min-height: 88px;
      resize: vertical;
      box-sizing: border-box;
      transition: border-color var(--gl-duration-base) var(--gl-ease);
    }
    textarea:focus-visible {
      outline: none;
      border-color: var(--gl-focus-ring-strong);
      box-shadow: 0 0 0 3px var(--gl-focus-ring);
    }
  `;

  static override properties = {
    label: { type: String },
    name: { type: String },
    value: { type: String },
    placeholder: { type: String },
    rows: { type: Number },
  };

  public label = "";
  public name = "";
  public value = "";
  public placeholder = "";
  public rows = 4;

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

  private onInput(e: Event): void {
    const t = e.target as HTMLTextAreaElement;
    this.value = t.value;
    this.dispatchEvent(new CustomEvent("gl-input", { detail: { value: t.value }, bubbles: true, composed: true }));
  }

  protected override render() {
    return html`
      <label>
        ${this.label ? html`<span>${this.label}</span>` : null}
        <textarea
          name=${this.name}
          .value=${this.value}
          placeholder=${this.placeholder}
          rows=${this.rows}
          @input=${(e: Event) => this.onInput(e)}
        ></textarea>
      </label>
    `;
  }
}

if (!customElements.get("gl-textarea")) {
  customElements.define("gl-textarea", GlTextarea);
}

declare global {
  interface HTMLElementTagNameMap {
    "gl-textarea": GlTextarea;
  }
}
