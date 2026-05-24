// <glossa-provider> — root element. Boots one SDK Client, owns the
// loaded bundle for the active locale, propagates updates via
// @lit/context, and listens to SSE for live patches.
//
// Public attributes (kebab-case):
//   project    — project slug (required)
//   locale     — BCP-47 locale code; changing it re-fetches
//   api-url    — Glossa API base URL
//   api-key    — Bearer token used by the SDK
//   strict     — boolean; logs missing-key warnings in dev
//
// Renders only the default <slot>. No wrapper markup → no
// landmark / semantic disruption.

import { ContextProvider } from "@lit/context";
import { LitElement, css, html } from "lit";

import { createClient, type Bundle, type Client } from "@felixgeelhaar/glossa-sdk";

import { glossaContext, type GlossaContextValue } from "./context.js";

export class GlossaProvider extends LitElement {
  static override styles = css`
    :host {
      /* Provider is a transparent container. display:contents lets
         the parent layout / semantics flow straight through to
         descendants without a stray wrapper box. */
      display: contents;
    }
  `;

  static override properties = {
    project: { type: String },
    locale: { type: String },
    apiUrl: { type: String, attribute: "api-url" },
    apiKey: { type: String, attribute: "api-key" },
    strict: { type: Boolean },
  };

  public project = "";
  public locale = "";
  public apiUrl = "";
  public apiKey = "";
  public strict = false;

  /** Test-only seam: lets a vitest override the fetch impl. */
  public fetchImpl: typeof fetch | undefined;

  private client: Client | undefined;
  private bundle: Bundle | null = null;
  private subscription: { close(): void } | undefined;
  private version = 0;

  private contextProvider = new ContextProvider(this, {
    context: glossaContext,
    initialValue: this.buildContext(),
  });

  public override connectedCallback(): void {
    super.connectedCallback();
    if (this.project && this.apiUrl && this.apiKey) {
      this.boot();
    }
  }

  public override disconnectedCallback(): void {
    super.disconnectedCallback();
    this.subscription?.close();
    this.subscription = undefined;
  }

  public override willUpdate(changed: Map<string, unknown>): void {
    super.willUpdate(changed);
    // Changing project / apiUrl / apiKey means a different SDK
    // client — re-boot. Changing only the locale just re-fetches.
    if (changed.has("project") || changed.has("apiUrl") || changed.has("apiKey")) {
      this.subscription?.close();
      this.subscription = undefined;
      this.client = undefined;
      this.bundle = null;
      if (this.project && this.apiUrl && this.apiKey) {
        this.boot();
      }
    } else if (changed.has("locale") && this.client) {
      void this.loadBundle();
    }
  }

  private boot(): void {
    this.client = createClient({
      project: this.project,
      apiKey: this.apiKey,
      apiUrl: this.apiUrl,
      ...(this.fetchImpl ? { fetch: this.fetchImpl } : {}),
    });
    void this.loadBundle();
    this.subscription = this.client.subscribe({
      onEvent: (e) => {
        // The SDK already patched its cache before invoking us;
        // mirror that into our local bundle reference and notify
        // consumers via a fresh context object.
        if (this.bundle && e.locale === this.locale) {
          this.bundle = {
            ...this.bundle,
            messages: { ...this.bundle.messages, [e.key]: e.value },
            statuses: { ...this.bundle.statuses, [e.key]: e.status },
          };
          this.publishContext();
        }
      },
    });
  }

  private async loadBundle(): Promise<void> {
    if (!this.client || !this.locale) return;
    try {
      this.bundle = await this.client.bundle(this.locale);
      this.publishContext();
    } catch (err) {
      // Network / auth failures don't crash the page — slot
      // content keeps rendering. Strict mode surfaces them so dev
      // builds catch broken configuration.
      if (this.strict) {
        // eslint-disable-next-line no-console
        console.warn("[glossa-provider] bundle load failed:", err);
      }
    }
  }

  private publishContext(): void {
    this.version++;
    this.contextProvider.setValue(this.buildContext());
  }

  private buildContext(): GlossaContextValue {
    const bundle = this.bundle;
    const strict = this.strict;
    return {
      locale: this.locale,
      strict,
      version: this.version,
      get(key: string): string | undefined {
        const v = bundle?.messages[key];
        if (v === undefined && strict) {
          // eslint-disable-next-line no-console
          console.warn(`[glossa] missing key: ${key}`);
        }
        return v;
      },
    };
  }

  protected override render() {
    return html`<slot></slot>`;
  }
}

if (!customElements.get("glossa-provider")) {
  customElements.define("glossa-provider", GlossaProvider);
}

declare global {
  interface HTMLElementTagNameMap {
    "glossa-provider": GlossaProvider;
  }
}
