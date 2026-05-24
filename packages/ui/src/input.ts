import { LitElement, css, html } from "lit";

/**
 * <gl-input label="…" type="email" name="email" required hint="…">
 *
 * Wraps a native <input>. Exposes the underlying value via the
 * standard form-associated API so a parent <form> picks it up with
 * FormData out of the box (parents read `fd.get("name")`).
 */
export class GlInput extends LitElement {
  // Form association — lets <input>'s name show up in FormData.
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
    input {
      font: inherit;
      font-family: var(--gl-font-ui);
      font-size: var(--gl-text-base);
      color: var(--gl-text);
      background: var(--gl-surface);
      border: 1px solid var(--gl-border);
      border-radius: var(--gl-radius-md);
      padding: 8px 10px;
      width: 100%;
      box-sizing: border-box;
      transition:
        border-color var(--gl-duration-base) var(--gl-ease),
        box-shadow var(--gl-duration-base) var(--gl-ease);
    }
    input::placeholder {
      color: var(--gl-text-subtle);
    }
    input:focus-visible {
      outline: none;
      border-color: var(--gl-focus-ring-strong);
      box-shadow: 0 0 0 3px var(--gl-focus-ring);
    }
    input[aria-invalid="true"] {
      border-color: var(--gl-danger);
    }
    .hint {
      font-size: var(--gl-text-xs);
      color: var(--gl-text-subtle);
    }
    .err {
      font-size: var(--gl-text-xs);
      color: var(--gl-danger);
    }
  `;

  static override properties = {
    label: { type: String },
    name: { type: String },
    type: { type: String },
    value: { type: String },
    placeholder: { type: String },
    required: { type: Boolean },
    autocomplete: { type: String },
    hint: { type: String },
    error: { type: String },
    readonly: { type: Boolean },
    mono: { type: Boolean },
  };

  public label = "";
  public name = "";
  public type: "text" | "email" | "password" | "url" | "number" = "text";
  public value = "";
  public placeholder = "";
  public required = false;
  public autocomplete = "";
  public hint = "";
  public error = "";
  public readonly = false;
  public mono = false;

  private internals: ElementInternals | null = null;

  public constructor() {
    super();
    if (typeof this.attachInternals === "function") {
      this.internals = this.attachInternals();
    }
  }

  public override willUpdate(changed: Map<string, unknown>): void {
    if (changed.has("value") && this.internals) {
      this.internals.setFormValue(this.value);
    }
  }

  private onInput(e: Event): void {
    const t = e.target as HTMLInputElement;
    this.value = t.value;
    this.dispatchEvent(new CustomEvent("gl-input", { detail: { value: t.value }, bubbles: true, composed: true }));
  }

  protected override render() {
    return html`
      <label>
        ${this.label ? html`<span>${this.label}</span>` : null}
        <input
          type=${this.type}
          name=${this.name}
          .value=${this.value}
          placeholder=${this.placeholder}
          ?required=${this.required}
          ?readonly=${this.readonly}
          autocomplete=${this.autocomplete || "off"}
          aria-invalid=${this.error ? "true" : "false"}
          aria-describedby=${this.hint || this.error ? "gl-input-desc" : ""}
          style=${this.mono ? "font-family: var(--gl-font-mono);" : ""}
          @input=${(e: Event) => this.onInput(e)}
          @change=${(e: Event) => this.onInput(e)}
        />
        ${this.error
          ? html`<span id="gl-input-desc" class="err">${this.error}</span>`
          : this.hint
            ? html`<span id="gl-input-desc" class="hint">${this.hint}</span>`
            : null}
      </label>
    `;
  }
}

if (!customElements.get("gl-input")) {
  customElements.define("gl-input", GlInput);
}

declare global {
  interface HTMLElementTagNameMap {
    "gl-input": GlInput;
  }
}
