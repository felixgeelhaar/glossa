// AI translator providers tab — list, add, test, delete.
//
// Surfaces tenant-scoped provider configs that drive the source-locale
// fan-out. The API key is write-only: server stores ciphertext, never
// returns plaintext, so an existing row's key field is empty + only
// sent on update if the admin re-enters it.

import { LitElement, css, html, unsafeCSS } from "lit";

import { glTableStyles, toast } from "@glossa/ui";

import type { adminClient, AIProviderRow } from "./api-client.js";

type Client = ReturnType<typeof adminClient>;

const KIND_OPTIONS = [
  { value: "openai", label: "OpenAI" },
  { value: "anthropic", label: "Anthropic" },
  { value: "gemini", label: "Gemini" },
  { value: "custom", label: "Custom (OpenAI-compatible)" },
];

export class GlossaAdminAIProvidersTab extends LitElement {
  static override styles = css`
    :host {
      display: block;
    }
    ${unsafeCSS(glTableStyles)}
    .row {
      display: grid;
      grid-template-columns: repeat(auto-fit, minmax(160px, 1fr));
      gap: var(--gl-space-3);
      align-items: end;
      margin-bottom: var(--gl-space-3);
    }
    .err {
      color: var(--gl-danger);
      font-size: var(--gl-text-sm);
    }
    .actions {
      display: flex;
      gap: var(--gl-space-2);
    }
    p.hint {
      color: var(--gl-text-dim);
      font-size: var(--gl-text-sm);
      margin: 0 0 var(--gl-space-3);
    }
  `;

  static override properties = {
    client: { state: true },
    rows: { state: true },
    err: { state: true },
    fKind: { state: true },
    fLabel: { state: true },
    fBaseURL: { state: true },
    fModel: { state: true },
    fKey: { state: true },
  };

  public client!: Client;
  public rows: AIProviderRow[] = [];
  public err = "";
  public fKind = "openai";
  public fLabel = "";
  public fBaseURL = "";
  public fModel = "";
  public fKey = "";

  public override willUpdate(changed: Map<string, unknown>): void {
    if (changed.has("client")) void this.load();
  }

  private async load(): Promise<void> {
    try {
      const res = await this.client.listAIProviders();
      this.rows = res.providers ?? [];
      this.err = "";
    } catch (e) {
      this.err = (e as Error).message;
    }
  }

  private async onCreate(e: Event): Promise<void> {
    e.preventDefault();
    try {
      await this.client.createAIProvider({
        kind: this.fKind,
        label: this.fLabel,
        baseUrl: this.fBaseURL || undefined,
        model: this.fModel,
        apiKey: this.fKey,
      });
      this.fLabel = "";
      this.fBaseURL = "";
      this.fModel = "";
      this.fKey = "";
      await this.load();
      toast("Provider added.", "ok");
    } catch (ex) {
      this.err = (ex as Error).message;
      toast(this.err, "err");
    }
  }

  private async onToggle(row: AIProviderRow): Promise<void> {
    try {
      await this.client.updateAIProvider(row.id, {
        label: row.label,
        baseUrl: row.baseUrl,
        model: row.model,
        enabled: !row.enabled,
      });
      await this.load();
    } catch (ex) {
      toast((ex as Error).message, "err");
    }
  }

  private async onTest(row: AIProviderRow): Promise<void> {
    const source = prompt(`Test ${row.label} — source text (de):`, "Guten Morgen.");
    if (source === null) return;
    try {
      const res = await this.client.testAIProvider(row.id, source);
      if (res.ok) {
        toast(`OK — ${res.provider}: ${res.translation}`, "ok");
      } else {
        toast(`Test failed: ${res.error}`, "err");
      }
    } catch (ex) {
      toast((ex as Error).message, "err");
    }
  }

  private async onDelete(row: AIProviderRow): Promise<void> {
    if (!confirm(`Delete provider ${row.label}?`)) return;
    try {
      await this.client.deleteAIProvider(row.id);
      await this.load();
      toast("Deleted.", "ok");
    } catch (ex) {
      toast((ex as Error).message, "err");
    }
  }

  protected override render() {
    return html`
      <p class="hint">
        Configured providers translate any source-locale write into every enabled non-source locale
        with status <strong>ai_translated</strong>. Reviewers approve or edit. Existing
        approved / needs_review translations are never overwritten.
      </p>
      ${this.err ? html`<p class="err" role="alert">${this.err}</p>` : null}
      <form class="row" @submit=${(e: Event) => void this.onCreate(e)}>
        <gl-select
          label="Kind"
          .value=${this.fKind}
          .options=${KIND_OPTIONS}
          @gl-change=${(e: CustomEvent<{ value: string }>) => {
            this.fKind = e.detail.value;
          }}
        ></gl-select>
        <gl-input
          label="Label"
          required
          placeholder="OpenAI prod"
          .value=${this.fLabel}
          @gl-input=${(e: CustomEvent<{ value: string }>) => {
            this.fLabel = e.detail.value;
          }}
        ></gl-input>
        <gl-input
          label="Model"
          required
          placeholder="gpt-4o-mini"
          .value=${this.fModel}
          @gl-input=${(e: CustomEvent<{ value: string }>) => {
            this.fModel = e.detail.value;
          }}
        ></gl-input>
        <gl-input
          label="Base URL (optional)"
          placeholder="https://api.openai.com"
          .value=${this.fBaseURL}
          @gl-input=${(e: CustomEvent<{ value: string }>) => {
            this.fBaseURL = e.detail.value;
          }}
        ></gl-input>
        <gl-input
          label="API key"
          type="password"
          required
          .value=${this.fKey}
          @gl-input=${(e: CustomEvent<{ value: string }>) => {
            this.fKey = e.detail.value;
          }}
        ></gl-input>
        <gl-button variant="primary" type="submit">Add provider</gl-button>
      </form>
      <table class="gl-table" role="grid">
        <thead>
          <tr><th>Kind</th><th>Label</th><th>Model</th><th>Enabled</th><th>Actions</th></tr>
        </thead>
        <tbody>
          ${this.rows.map(
            (p) => html`
              <tr>
                <td><gl-badge variant="accent">${p.kind}</gl-badge></td>
                <td>${p.label}</td>
                <td class="gl-cell-mono">${p.model}</td>
                <td>
                  ${p.enabled
                    ? html`<gl-badge variant="approved">on</gl-badge>`
                    : html`<gl-badge variant="neutral">off</gl-badge>`}
                </td>
                <td>
                  <div class="actions">
                    <gl-button size="sm" @click=${() => void this.onTest(p)}>Test</gl-button>
                    <gl-button size="sm" @click=${() => void this.onToggle(p)}>
                      ${p.enabled ? "Disable" : "Enable"}
                    </gl-button>
                    <gl-button size="sm" variant="danger" @click=${() => void this.onDelete(p)}>
                      Delete
                    </gl-button>
                  </div>
                </td>
              </tr>
            `,
          )}
        </tbody>
      </table>
    `;
  }
}

if (!customElements.get("glossa-admin-ai-providers-tab")) {
  customElements.define("glossa-admin-ai-providers-tab", GlossaAdminAIProvidersTab);
}

declare global {
  interface HTMLElementTagNameMap {
    "glossa-admin-ai-providers-tab": GlossaAdminAIProvidersTab;
  }
}
