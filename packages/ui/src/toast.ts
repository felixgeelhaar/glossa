// <gl-toast> — non-blocking confirmation / error notice. The
// helper `toast(msg, variant)` mounts one to <body>, auto-removes
// after the default 3s. ARIA live region announces to screen
// readers.

import { LitElement, css, html } from "lit";

export type ToastVariant = "ok" | "err";

export class GlToast extends LitElement {
  static override styles = css`
    :host {
      position: fixed;
      bottom: var(--gl-space-5);
      right: var(--gl-space-5);
      z-index: 9999;
    }
    .toast {
      background: var(--gl-surface-raised);
      color: var(--gl-text);
      border: 1px solid var(--gl-border);
      border-radius: var(--gl-radius-md);
      padding: 10px 14px;
      box-shadow: var(--gl-shadow-lg);
      max-width: 360px;
      font-size: var(--gl-text-md);
      animation: slide-in var(--gl-duration-slow) var(--gl-ease);
    }
    .err {
      border-color: var(--gl-danger);
      color: var(--gl-danger);
    }
    .ok {
      border-color: var(--gl-success);
      color: var(--gl-success);
    }
    @keyframes slide-in {
      from {
        opacity: 0;
        transform: translateY(8px);
      }
      to {
        opacity: 1;
        transform: translateY(0);
      }
    }
  `;

  static override properties = {
    variant: { type: String },
    message: { type: String },
  };

  public variant: ToastVariant = "ok";
  public message = "";

  protected override render() {
    return html`
      <div class=${`toast ${this.variant}`} role="status" aria-live="polite">
        ${this.message}
      </div>
    `;
  }
}

if (!customElements.get("gl-toast")) {
  customElements.define("gl-toast", GlToast);
}

/**
 * Imperative helper: pops a toast attached to <body>. Returns a
 * function that removes it immediately if the caller wants to
 * cancel.
 */
export function toast(message: string, variant: ToastVariant = "ok", ttl = 3000): () => void {
  if (typeof document === "undefined") return () => undefined;
  const el = document.createElement("gl-toast") as GlToast;
  el.message = message;
  el.variant = variant;
  document.body.appendChild(el);
  const timer = window.setTimeout(() => el.remove(), ttl);
  return () => {
    clearTimeout(timer);
    el.remove();
  };
}

declare global {
  interface HTMLElementTagNameMap {
    "gl-toast": GlToast;
  }
}
