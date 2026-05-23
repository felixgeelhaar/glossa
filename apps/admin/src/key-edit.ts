// <glossa-admin-key-edit> — single-key editor. ICU live preview
// runs the current value through @glossa/format using a sample
// `{count}` value the translator can change, so plural / select
// arms render against real data.
//
// Emits two events:
//   save   — { key, value, status }
//   cancel — no detail
//
// The parent (admin-app) owns the network round-trip; this
// component is pure UI.

import { LitElement, css, html } from "lit";

import { format } from "@glossa/format";
import type { TranslationStatus } from "@glossa/sdk";

export class GlossaAdminKeyEdit extends LitElement {
  static override styles = css`
    :host {
      display: block;
    }
    form {
      display: grid;
      gap: 12px;
    }
    label {
      display: grid;
      gap: 4px;
      font-size: 13px;
    }
    textarea,
    input,
    select {
      font: inherit;
      padding: 6px 8px;
      border: 1px solid currentColor;
      border-radius: 4px;
      background: transparent;
      color: inherit;
    }
    textarea {
      min-height: 80px;
    }
    .preview {
      padding: 8px 12px;
      border-left: 3px solid currentColor;
      font-style: italic;
    }
    .actions {
      display: flex;
      gap: 8px;
    }
    button {
      font: inherit;
      padding: 6px 12px;
      border: 1px solid currentColor;
      border-radius: 4px;
      background: transparent;
      color: inherit;
      cursor: pointer;
    }
    button[type="submit"] {
      font-weight: 600;
    }
  `;

  static override properties = {
    keyName: { type: String },
    value: { type: String },
    locale: { type: String },
    // Internal preview / form state.
    draftValue: { state: true },
    draftStatus: { state: true },
    sampleCount: { state: true },
  };

  public keyName = "";
  public value = "";
  public locale = "en";

  private draftValue = "";
  private draftStatus: TranslationStatus = "needs_review";
  private sampleCount = 2;

  public override willUpdate(changed: Map<string, unknown>): void {
    if (changed.has("value")) this.draftValue = this.value;
    if (changed.has("keyName")) this.draftStatus = "needs_review";
  }

  private renderPreview(): string {
    try {
      return format(this.draftValue, this.locale, { count: this.sampleCount, value: "female" });
    } catch (err) {
      return `[format error: ${(err as Error).message}]`;
    }
  }

  private onSubmit(e: Event): void {
    e.preventDefault();
    this.dispatchEvent(
      new CustomEvent("save", {
        detail: { key: this.keyName, value: this.draftValue, status: this.draftStatus },
        bubbles: true,
        composed: true,
      }),
    );
  }

  protected override render() {
    return html`
      <form @submit=${(e: Event) => this.onSubmit(e)}>
        <label>
          Key
          <input type="text" .value=${this.keyName} readonly aria-readonly="true" />
        </label>
        <label>
          Value (${this.locale})
          <textarea
            .value=${this.draftValue}
            @input=${(e: Event) => {
              this.draftValue = (e.target as HTMLTextAreaElement).value;
            }}
            aria-label="Translation value"
          ></textarea>
        </label>
        <div class="preview" aria-live="polite">
          Preview · count=${this.sampleCount} → ${this.renderPreview()}
        </div>
        <label>
          Sample count
          <input
            type="number"
            min="0"
            .value=${String(this.sampleCount)}
            @input=${(e: Event) => {
              this.sampleCount = Number((e.target as HTMLInputElement).value);
            }}
          />
        </label>
        <label>
          Status on save
          <select
            .value=${this.draftStatus}
            @change=${(e: Event) => {
              this.draftStatus = (e.target as HTMLSelectElement).value as TranslationStatus;
            }}
          >
            <option value="needs_review">needs_review</option>
            <option value="approved">approved</option>
            <option value="pending">pending</option>
          </select>
        </label>
        <div class="actions">
          <button type="submit">Save</button>
          <button
            type="button"
            @click=${() => this.dispatchEvent(new CustomEvent("cancel", { bubbles: true, composed: true }))}
          >
            Cancel
          </button>
        </div>
      </form>
    `;
  }
}

if (!customElements.get("glossa-admin-key-edit")) {
  customElements.define("glossa-admin-key-edit", GlossaAdminKeyEdit);
}

declare global {
  interface HTMLElementTagNameMap {
    "glossa-admin-key-edit": GlossaAdminKeyEdit;
  }
}
