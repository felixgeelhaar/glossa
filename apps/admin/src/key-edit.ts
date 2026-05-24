// <glossa-admin-key-edit> — single-key editor. Uses @glossa/ui
// primitives for inputs + buttons; ICU live preview via
// @glossa/format unchanged.

import { LitElement, css, html } from "lit";

import { format } from "@glossa/format";
import type { TranslationStatus } from "@glossa/sdk";

export class GlossaAdminKeyEdit extends LitElement {
  static override styles = css`
    :host { display: block; }
    form { display: grid; gap: var(--gl-space-3); }
    .preview {
      padding: var(--gl-space-3);
      background: var(--gl-surface-sunken);
      border-left: 3px solid var(--gl-accent);
      border-radius: 0 var(--gl-radius-md) var(--gl-radius-md) 0;
      color: var(--gl-text);
      font-size: var(--gl-text-md);
    }
    .preview-meta {
      color: var(--gl-text-muted);
      font-size: var(--gl-text-xs);
      font-family: var(--gl-font-mono);
    }
    .actions {
      display: flex;
      gap: var(--gl-space-2);
    }
    .grid-two {
      display: grid;
      grid-template-columns: 1fr 1fr;
      gap: var(--gl-space-3);
    }
  `;

  static override properties = {
    keyName: { type: String },
    value: { type: String },
    locale: { type: String },
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
        <gl-input label="Key" .value=${this.keyName} readonly mono></gl-input>
        <gl-textarea
          label=${`Value (${this.locale})`}
          .value=${this.draftValue}
          @gl-input=${(e: CustomEvent<{ value: string }>) => {
            this.draftValue = e.detail.value;
          }}
        ></gl-textarea>
        <div class="preview" aria-live="polite">
          ${this.renderPreview()}
          <div class="preview-meta">preview · count=${this.sampleCount}</div>
        </div>
        <div class="grid-two">
          <gl-input
            label="Sample count"
            type="number"
            .value=${String(this.sampleCount)}
            @gl-input=${(e: CustomEvent<{ value: string }>) => {
              this.sampleCount = Number(e.detail.value);
            }}
          ></gl-input>
          <gl-select
            label="Status on save"
            .value=${this.draftStatus}
            .options=${[
              { value: "needs_review", label: "needs_review" },
              { value: "approved", label: "approved" },
              { value: "pending", label: "pending" },
            ]}
            @gl-change=${(e: CustomEvent<{ value: string }>) => {
              this.draftStatus = e.detail.value as TranslationStatus;
            }}
          ></gl-select>
        </div>
        <div class="actions">
          <gl-button variant="primary" type="submit">Save</gl-button>
          <gl-button
            variant="ghost"
            @click=${() => this.dispatchEvent(new CustomEvent("cancel", { bubbles: true, composed: true }))}
          >
            Cancel
          </gl-button>
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
