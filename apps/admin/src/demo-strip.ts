// <glossa-admin-demo-strip> — mounts every public @felixgeelhaar/glossa-elements
// custom element on the same <glossa-provider> so any SSE update
// arriving from the editor flows through the SDK cache into the
// rendered strings. Lives in the admin page so the translator
// sees their edit propagate live to a real consumer surface
// without opening a second browser tab.

import { LitElement, css, html } from "lit";

export class GlossaAdminDemoStrip extends LitElement {
  static override styles = css`
    :host {
      display: block;
      font-size: 14px;
    }
    dl {
      display: grid;
      grid-template-columns: max-content 1fr;
      gap: 4px 12px;
      margin: 0;
    }
    dt {
      font-weight: 600;
    }
  `;

  static override properties = {
    apiUrl: { type: String },
    apiKey: { type: String },
    project: { type: String },
    locale: { type: String },
  };

  public apiUrl = "";
  public apiKey = "";
  public project = "";
  public locale = "";

  protected override render() {
    return html`
      <glossa-provider
        api-url=${this.apiUrl}
        api-key=${this.apiKey}
        project=${this.project}
        locale=${this.locale}
      >
        <dl>
          <dt>text</dt>
          <dd>
            <glossa-text key="cart.checkout">Approve plan</glossa-text>
          </dd>
          <dt>rich</dt>
          <dd>
            <glossa-rich key="athlete.greeting" vars='{"name":"Sophia"}'>Hi, Sophia!</glossa-rich>
          </dd>
          <dt>plural</dt>
          <dd>
            <glossa-plural key="athlete.session_count" count="3">no sessions</glossa-plural>
          </dd>
          <dt>select</dt>
          <dd>
            <glossa-select key="user.gender" value="female">they</glossa-select>
          </dd>
        </dl>
      </glossa-provider>
    `;
  }
}

if (!customElements.get("glossa-admin-demo-strip")) {
  customElements.define("glossa-admin-demo-strip", GlossaAdminDemoStrip);
}

declare global {
  interface HTMLElementTagNameMap {
    "glossa-admin-demo-strip": GlossaAdminDemoStrip;
  }
}
