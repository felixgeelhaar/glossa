// <glossa-selector> — language picker that integrates with
// <glossa-provider>. Reads the active locale from glossaContext,
// renders a styled <select> (replaceable via the default slot), and
// dispatches a "glossa-locale-change" CustomEvent when the user
// picks a different locale.
//
// The selector does NOT mutate the provider's locale attribute
// directly. Instead it emits an event the application listens to.
// This keeps the persistence decision (cookie / localStorage / user
// profile column) in app code where it belongs — the library doesn't
// know which transport the app uses.
//
// Public attributes (kebab-case):
//   locales        Comma-separated BCP-47 codes, e.g. "en,de,fr".
//                  When omitted, the selector renders only the
//                  current locale (read-only).
//   labels         Comma-separated human labels matching `locales`
//                  order. When omitted, the BCP-47 code is shown.
//                  Example: "English,Deutsch,Français".
//   label          Accessible label for the control. Default
//                  "Language". Localise in app code if needed.
//   auto-detect    Boolean. On first connect, if the user's browser
//                  language differs from the current provider
//                  locale AND is in `locales`, the selector dispatches
//                  a glossa-locale-change event with source="auto".
//                  App code decides whether to honour the
//                  suggestion (typically only when the user has not
//                  yet persisted an explicit pick).
//   disabled       Boolean. Renders the select as disabled — useful
//                  during the persistence round-trip.
//
// Events:
//   glossa-locale-change   detail: { locale, source: "manual" | "auto" }
//                          Bubbles + composed so listeners outside
//                          the shadow DOM tree receive it.
//
// Slots:
//   default        Replace the entire built-in <select>. Slotted
//                  content must dispatch its own glossa-locale-change
//                  event to drive the provider; the selector forwards
//                  it untouched.
//
// Example:
//   <glossa-provider project="pet-medical" locale="de" ...>
//     <glossa-selector
//       locales="en,de"
//       labels="English,Deutsch"
//       label="Sprache"
//       auto-detect
//     ></glossa-selector>
//   </glossa-provider>
//
//   <script>
//     document.addEventListener("glossa-locale-change", (e) => {
//       // App-level persistence + apply to provider.
//       savePreference(e.detail.locale);
//       document.querySelector("glossa-provider")
//         ?.setAttribute("locale", e.detail.locale);
//     });
//   </script>

import { ContextConsumer } from "@lit/context";
import { LitElement, css, html, nothing } from "lit";

import { glossaContext, type GlossaContextValue } from "./context.js";

/**
 * Detail payload of the glossa-locale-change CustomEvent.
 *
 * `source: "manual"` — the user picked the locale via the control.
 * `source: "auto"`   — the auto-detect path is suggesting a locale.
 *                       Apps should usually only honour this when no
 *                       explicit user preference exists.
 */
export interface GlossaLocaleChangeDetail {
  locale: string;
  source: "manual" | "auto";
}

export class GlossaSelector extends LitElement {
  static override styles = css`
    :host {
      display: inline-block;
    }
    select {
      font: inherit;
      color: inherit;
      background: inherit;
      padding: 0.25em 0.5em;
      border: 1px solid currentColor;
      border-radius: 0.25em;
    }
    select:focus-visible {
      outline: 2px solid currentColor;
      outline-offset: 2px;
    }
  `;

  static override properties = {
    locales: { type: String },
    labels: { type: String },
    label: { type: String },
    autoDetect: { type: Boolean, attribute: "auto-detect" },
    disabled: { type: Boolean },
  };

  public locales = "";
  public labels = "";
  public label = "Language";
  public autoDetect = false;
  public disabled = false;

  /** Test-only seam — lets vitest stub the browser-language source. */
  public detectImpl: (() => string | undefined) | undefined;

  private ctx = new ContextConsumer<typeof glossaContext, this>(this, {
    context: glossaContext,
    subscribe: true,
  });

  private autoDetected = false;

  public override connectedCallback(): void {
    super.connectedCallback();
    // Defer the auto-detect to the next microtask so the provider
    // has a chance to publish its initial context value first. If
    // we fire too early, current locale reads as "" and we'd
    // suggest a locale even when the provider is about to load
    // the user's preference.
    queueMicrotask(() => this.maybeAutoDetect());
  }

  public override willUpdate(changed: Map<string, unknown>): void {
    super.willUpdate(changed);
    // Re-run auto-detect when the locales list changes — a longer
    // list might suddenly include the browser's language.
    if (changed.has("locales") || changed.has("autoDetect")) {
      this.autoDetected = false;
      this.maybeAutoDetect();
    }
  }

  protected override render() {
    const ctx = this.ctx.value;
    const current = ctx?.locale ?? "";
    const locales = this.parsedLocales();
    const labels = this.parsedLabels();

    // No locales attribute → render the current locale read-only.
    // Useful as a status indicator before the app is ready to ship
    // multiple locales.
    if (locales.length === 0) {
      return html`<span aria-label=${this.label}>${current || nothing}</span>`;
    }

    // Slotted custom UI wins — pass slot through and trust the
    // consumer to dispatch glossa-locale-change.
    return html`
      <slot @slotchange=${this.handleSlotChange}>
        <label>
          <span class="sr-only">${this.label}</span>
          <select
            aria-label=${this.label}
            ?disabled=${this.disabled}
            .value=${current}
            @change=${this.handleChange}
          >
            ${locales.map((code, i) => {
              const labelText = labels[i] ?? code;
              return html`<option value=${code} ?selected=${code === current}>${labelText}</option>`;
            })}
          </select>
        </label>
      </slot>
    `;
  }

  private handleChange = (e: Event): void => {
    const target = e.target as HTMLSelectElement;
    this.dispatchLocaleChange(target.value, "manual");
  };

  private handleSlotChange = (): void => {
    // No-op for now; reserved so we can wire forwarding of nested
    // change events if slotted UIs become common.
  };

  /**
   * Detect the browser's preferred locale + emit a "auto" suggestion
   * iff (a) auto-detect is enabled, (b) we haven't already
   * suggested this session, (c) the detected locale is in the
   * configured `locales` list, and (d) it differs from the current
   * provider locale.
   */
  private maybeAutoDetect(): void {
    if (!this.autoDetect || this.autoDetected) return;
    const ctx = this.ctx.value;
    if (!ctx) return;
    const locales = this.parsedLocales();
    if (locales.length === 0) return;

    const browser = this.detectBrowserLocale();
    if (!browser) return;

    const matched = pickBestMatch(browser, locales);
    if (!matched) return;
    if (matched === ctx.locale) {
      this.autoDetected = true;
      return;
    }
    this.autoDetected = true;
    this.dispatchLocaleChange(matched, "auto");
  }

  private detectBrowserLocale(): string | undefined {
    if (this.detectImpl) return this.detectImpl();
    if (typeof navigator === "undefined") return undefined;
    return navigator.language || (navigator.languages && navigator.languages[0]);
  }

  private dispatchLocaleChange(locale: string, source: "manual" | "auto"): void {
    this.dispatchEvent(
      new CustomEvent<GlossaLocaleChangeDetail>("glossa-locale-change", {
        detail: { locale, source },
        bubbles: true,
        composed: true,
      }),
    );
  }

  private parsedLocales(): string[] {
    return splitCSV(this.locales);
  }

  private parsedLabels(): string[] {
    return splitCSV(this.labels);
  }
}

/**
 * pickBestMatch returns the configured locale that best matches the
 * browser-supplied BCP-47 code. Exact match wins; otherwise the
 * primary language tag is compared ("en-GB" matches "en" in the list).
 * Returns undefined when no overlap exists.
 */
export function pickBestMatch(browser: string, available: string[]): string | undefined {
  const b = browser.toLowerCase();
  for (const code of available) {
    if (code.toLowerCase() === b) return code;
  }
  const primary = b.split("-")[0];
  for (const code of available) {
    if (code.toLowerCase().split("-")[0] === primary) return code;
  }
  return undefined;
}

function splitCSV(raw: string): string[] {
  if (!raw) return [];
  return raw
    .split(",")
    .map((s) => s.trim())
    .filter(Boolean);
}

if (!customElements.get("glossa-selector")) {
  customElements.define("glossa-selector", GlossaSelector);
}

declare global {
  interface HTMLElementTagNameMap {
    "glossa-selector": GlossaSelector;
  }
}
