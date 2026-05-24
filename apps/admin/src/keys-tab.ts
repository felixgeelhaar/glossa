// Per-project API key management. Lists every key with its scope +
// label, allows issuing new keys with read or write scope, and
// revoking existing ones.
//
// The reveal-once API key after create is rendered inline (not via
// a parent modal) so the user never navigates away mid-copy.

import { LitElement, css, html, unsafeCSS } from "lit";

import { glTableStyles, toast } from "@felixgeelhaar/glossa-ui";

import type { adminClient, ProjectApiKeyRow } from "./api-client.js";

type Client = ReturnType<typeof adminClient>;

export class GlossaAdminKeysTab extends LitElement {
  static override styles = css`
    :host {
      display: block;
    }
    ${unsafeCSS(glTableStyles)}
    .row {
      display: grid;
      grid-template-columns: 1fr 160px auto;
      gap: var(--gl-space-3);
      align-items: end;
      margin-bottom: var(--gl-space-3);
    }
    .err {
      color: var(--gl-danger);
      font-size: var(--gl-text-sm);
    }
    .hint {
      color: var(--gl-text-muted);
      font-size: var(--gl-text-sm);
      margin: 0 0 var(--gl-space-3);
    }
    .reveal {
      padding: var(--gl-space-3) var(--gl-space-4);
      background: var(--gl-surface-elevated, var(--gl-surface));
      border: 1px solid var(--gl-accent);
      border-radius: var(--gl-radius-md);
      margin: var(--gl-space-3) 0;
      display: flex;
      flex-direction: column;
      gap: var(--gl-space-2);
    }
    .reveal-actions {
      display: flex;
      gap: var(--gl-space-2);
    }
    .actions {
      display: flex;
      gap: var(--gl-space-2);
    }
    .revoked {
      opacity: 0.55;
    }
  `;

  static override properties = {
    client: { state: true },
    slug: { state: true },
    rows: { state: true },
    err: { state: true },
    fLabel: { state: true },
    fScope: { state: true },
    pending: { state: true },
    revealedKey: { state: true },
    revealedLabel: { state: true },
  };

  public client!: Client;
  public slug = "";
  public rows: ProjectApiKeyRow[] = [];
  public err = "";
  public fLabel = "";
  public fScope: "read" | "write" = "read";
  public pending = false;
  public revealedKey = "";
  public revealedLabel = "";

  public override willUpdate(changed: Map<string, unknown>): void {
    if (changed.has("client") || changed.has("slug")) void this.load();
  }

  private async load(): Promise<void> {
    if (!this.client || !this.slug) return;
    try {
      const res = await this.client.listProjectApiKeys(this.slug);
      this.rows = res.keys ?? [];
      this.err = "";
    } catch (e) {
      this.err = (e as Error).message;
    }
  }

  private async onIssue(e: Event): Promise<void> {
    e.preventDefault();
    this.pending = true;
    this.err = "";
    this.requestUpdate();
    try {
      const out = await this.client.issueProjectApiKey(this.slug, {
        scope: this.fScope,
        label: this.fLabel.trim(),
      });
      this.revealedKey = out.apiKey;
      this.revealedLabel = `${out.key.label} (${out.key.scope})`;
      this.fLabel = "";
      await this.load();
      toast("Key created.", "ok");
    } catch (ex) {
      this.err = (ex as Error).message;
      toast(this.err, "err");
    } finally {
      this.pending = false;
      this.requestUpdate();
    }
  }

  private async onRevoke(row: ProjectApiKeyRow): Promise<void> {
    if (!confirm(`Revoke key "${row.label}" (${row.scope})? Consumers using it will start getting 401s.`)) return;
    try {
      await this.client.revokeProjectApiKey(this.slug, row.id);
      await this.load();
      toast("Key revoked.", "ok");
    } catch (ex) {
      toast((ex as Error).message, "err");
    }
  }

  private async copyRevealed(): Promise<void> {
    if (!this.revealedKey) return;
    try {
      await navigator.clipboard.writeText(this.revealedKey);
      toast("Copied.", "ok");
    } catch {
      /* clipboard blocked; the input is already select-all-able */
    }
  }

  private dismissRevealed(): void {
    this.revealedKey = "";
    this.revealedLabel = "";
    this.requestUpdate();
  }

  protected override render() {
    return html`
      <p class="hint">
        Issue one key per consumer + scope. <strong>read</strong> keys cover bundle fetch and SSE
        live updates — safe to embed in static / browser bundles. <strong>write</strong> keys also
        unlock translation edits + key scanning — keep them on a trusted server. Server stores only
        the SHA-256 hash; revealed value is only shown once.
      </p>

      ${this.revealedKey
        ? html`<div class="reveal" role="status">
            <div>
              <strong>API key for ${this.revealedLabel}</strong>
              — copy now, server-side hash is the only persistent copy.
            </div>
            <gl-input label="Key" readonly .value=${this.revealedKey}></gl-input>
            <div class="reveal-actions">
              <gl-button variant="primary" @click=${() => void this.copyRevealed()}>Copy</gl-button>
              <gl-button variant="ghost" @click=${() => this.dismissRevealed()}>I've saved it</gl-button>
            </div>
          </div>`
        : null}

      ${this.err ? html`<p class="err" role="alert">${this.err}</p>` : null}

      <form class="row" @submit=${(e: Event) => void this.onIssue(e)}>
        <gl-input
          label="Label"
          required
          placeholder="brotwerk web client"
          .value=${this.fLabel}
          @gl-input=${(e: CustomEvent<{ value: string }>) => {
            this.fLabel = e.detail.value;
          }}
        ></gl-input>
        <gl-select
          label="Scope"
          .value=${this.fScope}
          .options=${[
            { value: "read", label: "read (consumer)" },
            { value: "write", label: "write (server / CLI)" },
          ]}
          @gl-change=${(e: CustomEvent<{ value: string }>) => {
            this.fScope = e.detail.value as "read" | "write";
          }}
        ></gl-select>
        <gl-button variant="primary" type="submit" ?disabled=${this.pending}>
          ${this.pending ? "Issuing…" : "Issue key"}
        </gl-button>
      </form>

      <table class="gl-table" role="grid">
        <thead>
          <tr>
            <th>Label</th>
            <th>Scope</th>
            <th>Created</th>
            <th>Last used</th>
            <th>Actions</th>
          </tr>
        </thead>
        <tbody>
          ${this.rows.map(
            (k) => html`
              <tr class=${k.revokedAt ? "revoked" : ""}>
                <td>${k.label}</td>
                <td>
                  ${k.scope === "write"
                    ? html`<gl-badge variant="accent">write</gl-badge>`
                    : html`<gl-badge>read</gl-badge>`}
                </td>
                <td class="gl-cell-mono">${new Date(k.createdAt).toLocaleString()}</td>
                <td class="gl-cell-mono">
                  ${k.lastUsedAt ? new Date(k.lastUsedAt).toLocaleString() : "—"}
                </td>
                <td>
                  <div class="actions">
                    ${k.revokedAt
                      ? html`<gl-badge variant="danger">revoked</gl-badge>`
                      : html`<gl-button size="sm" variant="danger" @click=${() => void this.onRevoke(k)}>
                          Revoke
                        </gl-button>`}
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

if (!customElements.get("glossa-admin-keys-tab")) {
  customElements.define("glossa-admin-keys-tab", GlossaAdminKeysTab);
}

declare global {
  interface HTMLElementTagNameMap {
    "glossa-admin-keys-tab": GlossaAdminKeysTab;
  }
}
