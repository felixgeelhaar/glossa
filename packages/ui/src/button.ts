import { LitElement, css, html } from "lit";

export type ButtonVariant = "primary" | "ghost" | "danger" | "outline";
export type ButtonSize = "sm" | "md";

export class GlButton extends LitElement {
  static override styles = css`
    :host {
      display: inline-block;
    }
    button {
      font: inherit;
      font-family: var(--gl-font-ui);
      font-weight: 500;
      border-radius: var(--gl-radius-md);
      border: 1px solid transparent;
      cursor: pointer;
      transition:
        background var(--gl-duration-base) var(--gl-ease),
        border-color var(--gl-duration-base) var(--gl-ease),
        color var(--gl-duration-base) var(--gl-ease);
      display: inline-flex;
      align-items: center;
      gap: var(--gl-space-2);
      white-space: nowrap;
    }
    button:focus-visible {
      outline: 2px solid var(--gl-focus-ring-strong);
      outline-offset: 2px;
    }
    button:disabled {
      opacity: 0.5;
      cursor: not-allowed;
    }

    .sm {
      font-size: var(--gl-text-sm);
      padding: 4px 10px;
      height: 28px;
    }
    .md {
      font-size: var(--gl-text-base);
      padding: 6px 12px;
      height: 32px;
    }

    .primary {
      background: var(--gl-accent);
      color: var(--gl-accent-fg);
    }
    .primary:hover:not(:disabled) {
      background: var(--gl-accent-hover);
    }

    .outline {
      background: var(--gl-surface);
      color: var(--gl-text);
      border-color: var(--gl-border);
    }
    .outline:hover:not(:disabled) {
      border-color: var(--gl-border-strong);
      background: var(--gl-surface-raised);
    }

    .ghost {
      background: transparent;
      color: var(--gl-text);
    }
    .ghost:hover:not(:disabled) {
      background: var(--gl-surface-raised);
    }

    .danger {
      background: transparent;
      color: var(--gl-danger);
      border-color: var(--gl-border);
    }
    .danger:hover:not(:disabled) {
      background: var(--gl-danger-bg);
      border-color: var(--gl-danger);
    }
  `;

  static override properties = {
    variant: { type: String },
    size: { type: String },
    type: { type: String },
    disabled: { type: Boolean, reflect: true },
  };

  public variant: ButtonVariant = "outline";
  public size: ButtonSize = "md";
  public type: "button" | "submit" | "reset" = "button";
  public disabled = false;

  protected override render() {
    return html`
      <button
        type=${this.type}
        class=${`${this.size} ${this.variant}`}
        ?disabled=${this.disabled}
        @click=${(e: Event) => {
          if (this.type === "submit") {
            // Native <button type=submit> inside a shadow root
            // doesn't submit the outer form. Re-dispatch the
            // submit on the host's closest form so consumers can
            // <form @submit>.
            const form = this.closest("form");
            if (form) {
              e.preventDefault();
              form.requestSubmit();
            }
          }
        }}
      >
        <slot></slot>
      </button>
    `;
  }
}

if (!customElements.get("gl-button")) {
  customElements.define("gl-button", GlButton);
}

declare global {
  interface HTMLElementTagNameMap {
    "gl-button": GlButton;
  }
}
