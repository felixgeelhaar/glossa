// Editor tab — locale picker + key-list + edit form. Reuses the
// key-list / key-edit components from the earlier admin pass.

import { LitElement, css, html } from "lit";

import "./key-edit.js";
import "./key-list.js";

import type { adminClient, BundleResponse } from "./api-client.js";
import type { GlossaAdminKeyList } from "./key-list.js";
import type { GlossaAdminKeyEdit } from "./key-edit.js";

type Client = ReturnType<typeof adminClient>;

export class GlossaAdminEditorTab extends LitElement {
  static override styles = css`
    :host { display: block; }
    header { display: flex; gap: 12px; align-items: center; margin-bottom: 12px; }
    .err { color: #b00020; font-size: 13px; }
  `;

  static override properties = {
    client: { state: true },
    slug: { state: true },
    userRole: { state: true },
    scopedLocales: { state: true },
    locales: { state: true },
    locale: { state: true },
    bundle: { state: true },
    editing: { state: true },
    err: { state: true },
  };

  public client!: Client;
  public slug = "";
  public userRole: "admin" | "translator" = "admin";
  public scopedLocales: string[] = [];
  public locales: { id: string; code: string; label: string }[] = [];
  public locale = "";
  public bundle: BundleResponse | null = null;
  public editing: string | null = null;
  public err = "";

  public override willUpdate(changed: Map<string, unknown>): void {
    if (changed.has("client") || changed.has("slug")) {
      void this.loadLocales();
    }
  }

  private async loadLocales(): Promise<void> {
    if (!this.client || !this.slug) return;
    try {
      const all = await this.client.listLocales(this.slug);
      // Translator scoping: hide locales the user isn't assigned.
      const filtered = this.userRole === "translator" && this.scopedLocales.length > 0
        ? all.filter((l) => this.scopedLocales.includes(l.code))
        : all;
      this.locales = filtered;
      if (!this.locale && filtered[0]) this.locale = filtered[0].code;
      if (this.locale) await this.loadBundle();
    } catch (e) {
      this.err = (e as Error).message;
    }
  }

  private async loadBundle(): Promise<void> {
    if (!this.client || !this.slug || !this.locale) return;
    try {
      this.bundle = await this.client.listBundle(this.slug, this.locale);
      this.err = "";
    } catch (e) {
      this.err = (e as Error).message;
    }
  }

  private async onSave(detail: { key: string; value: string; status?: string }): Promise<void> {
    try {
      await this.client.patchTranslation(this.slug, this.locale, detail.key, {
        value: detail.value,
        status: detail.status,
      });
      // Optimistic local patch.
      if (this.bundle) {
        this.bundle = {
          ...this.bundle,
          messages: { ...this.bundle.messages, [detail.key]: detail.value },
          statuses: {
            ...this.bundle.statuses,
            [detail.key]: (detail.status as "pending" | "needs_review" | "approved") ?? "needs_review",
          },
        };
      }
      this.editing = null;
      this.requestUpdate();
    } catch (e) {
      this.err = (e as Error).message;
    }
  }

  protected override render() {
    return html`
      <header>
        <gl-select
          label="Locale"
          .value=${this.locale}
          .options=${this.locales.map((l) => ({ value: l.code, label: `${l.label} (${l.code})` }))}
          @gl-change=${(e: CustomEvent<{ value: string }>) => {
            this.locale = e.detail.value;
            void this.loadBundle();
          }}
        ></gl-select>
      </header>
      ${this.err ? html`<p class="err" role="alert">${this.err}</p>` : null}
      <glossa-admin-key-list
        .messages=${this.bundle?.messages ?? {}}
        .statuses=${this.bundle?.statuses ?? {}}
        .selected=${this.editing}
        @select-key=${(e: CustomEvent<{ key: string }>) => {
          this.editing = e.detail.key;
          this.requestUpdate();
        }}
      ></glossa-admin-key-list>
      ${this.editing && this.bundle
        ? html`
            <glossa-admin-key-edit
              .keyName=${this.editing}
              .value=${this.bundle.messages[this.editing] ?? ""}
              .locale=${this.locale}
              @cancel=${() => {
                this.editing = null;
                this.requestUpdate();
              }}
              @save=${(e: CustomEvent<{ key: string; value: string; status?: string }>) => void this.onSave(e.detail)}
            ></glossa-admin-key-edit>
          `
        : null}
    `;
  }

  // Avoid TS warning about unused imports — these are referenced
  // only via the tagged HTML templates which Lit reads at runtime.
  private _refs = { _l: undefined as GlossaAdminKeyList | undefined, _e: undefined as GlossaAdminKeyEdit | undefined };
}

if (!customElements.get("glossa-admin-editor-tab")) {
  customElements.define("glossa-admin-editor-tab", GlossaAdminEditorTab);
}

declare global {
  interface HTMLElementTagNameMap {
    "glossa-admin-editor-tab": GlossaAdminEditorTab;
  }
}
