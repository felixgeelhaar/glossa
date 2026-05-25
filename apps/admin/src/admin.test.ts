import { afterEach, describe, expect, it } from "vitest";

import type { BundleResponse } from "./api-client.js";
import "@felixgeelhaar/glossa-elements";

import "./admin-app.js";
import "./audit-tab.js";
import "./bulk-tab.js";
import "./diff-tab.js";
import "./editor-tab.js";
import "./key-edit.js";
import "./key-list.js";
import "./locales-tab.js";
import "./users-tab.js";
import type { GlossaAdmin } from "./admin-app.js";
import type { GlossaAdminEditorTab } from "./editor-tab.js";
import type { GlossaAdminKeyEdit } from "./key-edit.js";
import type { GlossaAdminKeyList } from "./key-list.js";
import type { GlossaAdminBulkTab } from "./bulk-tab.js";
import type { GlossaAdminDiffTab } from "./diff-tab.js";
import type { GlossaAdminLocalesTab } from "./locales-tab.js";
import type { GlossaAdminUsersTab } from "./users-tab.js";
import type { GlossaAdminAuditTab } from "./audit-tab.js";

const bundle: BundleResponse = {
  project: "demo",
  locale: "de",
  messages: { "cart.checkout": "Zur Kasse" },
  statuses: { "cart.checkout": "approved" },
};

async function flush(): Promise<void> {
  for (let i = 0; i < 30; i++) {
    await new Promise((r) => setTimeout(r, 0));
  }
}

function makeAuth(role: "admin" | "translator" = "admin"): typeof localStorage extends Storage ? void : never {
  localStorage.setItem(
    "glossa-admin-auth-v2",
    JSON.stringify({
      token: "fake.jwt.token",
      expires: new Date(Date.now() + 60 * 60 * 1000).toISOString(),
      user: { id: "u1", email: "felix@example.com", role, locales: [] },
      tenant: { id: "t1", slug: "demo", name: "Demo" },
    }),
  );
  localStorage.setItem("glossa-admin-api-url-v2", "https://glossa.test");
  return undefined as never;
}

afterEach(() => {
  for (const child of Array.from(document.body.children)) {
    document.body.removeChild(child);
  }
  localStorage.clear();
});

describe("<glossa-admin> login surface", () => {
  it("renders the JWT login form when no auth is stored", async () => {
    const el = document.createElement("glossa-admin") as GlossaAdmin;
    document.body.appendChild(el);
    await el.updateComplete;
    const form = el.shadowRoot!.querySelector("form");
    expect(form).toBeTruthy();
    // Tenant field removed in favour of /auth/discover. Email +
    // Password are the only credentials the form asks for.
    expect(form?.innerHTML).toContain("Email");
    expect(form?.innerHTML).toContain("Password");
    expect(form?.innerHTML).not.toContain('label="Tenant"');
  });

  it("skips the login form when a valid token is stored", async () => {
    makeAuth("admin");
    let listCount = 0;
    const fetchImpl = (async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url.endsWith("/admin/projects") && (listCount === 0)) {
        listCount++;
        return new Response(JSON.stringify([{ id: "p1", slug: "demo", name: "Demo", defaultLocale: "de" }]), {
          status: 200,
        });
      }
      return new Response("[]", { status: 200 });
    }) as typeof fetch;

    const el = document.createElement("glossa-admin") as GlossaAdmin;
    el.fetchImpl = fetchImpl;
    document.body.appendChild(el);
    await flush();
    await el.updateComplete;

    // After login the SPA renders <gl-tabs>; the bare <nav.tabs>
    // moved inside the primitive's shadow root, so we assert the
    // host element instead.
    const tabs = el.shadowRoot!.querySelector("gl-tabs");
    expect(tabs).toBeTruthy();
  });
});

describe("<glossa-admin-editor-tab>", () => {
  it("loads + renders the bundle through a stub client", async () => {
    const fakeClient = {
      listLocales: async () => [{ id: "l1", code: "de", label: "Deutsch", enabled: true }],
      listBundle: async () => bundle,
      patchTranslation: async () => ({ id: "t1", value: "X", status: "needs_review" }),
    } as unknown as GlossaAdminEditorTab["client"];

    const el = document.createElement("glossa-admin-editor-tab") as GlossaAdminEditorTab;
    el.client = fakeClient;
    el.slug = "demo";
    el.userRole = "admin";
    document.body.appendChild(el);
    await flush();
    await el.updateComplete;
    await flush();

    const list = el.shadowRoot!.querySelector("glossa-admin-key-list") as GlossaAdminKeyList;
    expect(list).toBeTruthy();
    await list.updateComplete;
    const rows = list.shadowRoot!.querySelectorAll("tbody tr");
    expect(rows.length).toBe(1);
  });
});

describe("<glossa-admin-key-edit>", () => {
  it("renders a live ICU preview against the draft value", async () => {
    const el = document.createElement("glossa-admin-key-edit") as GlossaAdminKeyEdit;
    el.keyName = "athlete.session_count";
    el.locale = "de";
    el.value = "{count, plural, one {Eine Einheit} other {# Einheiten}}";
    document.body.appendChild(el);
    await el.updateComplete;
    await flush();
    const preview = el.shadowRoot!.querySelector(".preview") as HTMLElement;
    expect(preview.textContent).toContain("2 Einheiten");
  });
});

describe("<glossa-admin-diff-tab>", () => {
  it("renders one row per locale", async () => {
    const fakeClient = {
      diff: async () => ({
        project: "demo",
        locales: [
          { locale: "de", label: "Deutsch", total: 5, pending: 1, needsReview: 2, approved: 2 },
          { locale: "en", label: "English", total: 5, pending: 0, needsReview: 1, approved: 4 },
        ],
      }),
    } as unknown as GlossaAdminDiffTab["client"];
    const el = document.createElement("glossa-admin-diff-tab") as GlossaAdminDiffTab;
    el.client = fakeClient;
    el.slug = "demo";
    document.body.appendChild(el);
    await flush();
    await el.updateComplete;
    const rows = el.shadowRoot!.querySelectorAll(".locale-row:not(.head)");
    expect(rows.length).toBe(2);
    const cells = el.shadowRoot!.querySelectorAll(".locale-row:not(.head) .cell");
    expect(cells.length).toBe(2 * 2);
    // de has pending=1 + needsReview=2 → pending cell carries pill
    const pills = el.shadowRoot!.querySelectorAll(".needs-review-pill");
    expect(pills.length).toBeGreaterThanOrEqual(1);
  });

  it("collapses to an Up-to-date cell when a locale has no work", async () => {
    const fakeClient = {
      diff: async () => ({
        project: "demo",
        locales: [{ locale: "de", label: "Deutsch", total: 0, pending: 0, needsReview: 0, approved: 0 }],
      }),
    } as unknown as GlossaAdminDiffTab["client"];
    const el = document.createElement("glossa-admin-diff-tab") as GlossaAdminDiffTab;
    el.client = fakeClient;
    el.slug = "demo";
    document.body.appendChild(el);
    await flush();
    await el.updateComplete;
    const up = el.shadowRoot!.querySelector(".uptodate");
    expect(up).toBeTruthy();
  });

  it("renders a locale code badge in the meta column", async () => {
    const fakeClient = {
      diff: async () => ({
        project: "demo",
        locales: [{ locale: "fr", label: "Français", total: 3, pending: 3, needsReview: 0, approved: 0 }],
      }),
    } as unknown as GlossaAdminDiffTab["client"];
    const el = document.createElement("glossa-admin-diff-tab") as GlossaAdminDiffTab;
    el.client = fakeClient;
    el.slug = "demo";
    document.body.appendChild(el);
    await flush();
    await el.updateComplete;
    const code = el.shadowRoot!.querySelector(".meta .code");
    expect(code?.textContent).toBe("fr");
  });
});

describe("<glossa-admin-locales-tab>", () => {
  it("lists locales + supports toggle", async () => {
    let toggled = false;
    const fakeClient = {
      listLocales: async () => [{ id: "l1", code: "de", label: "Deutsch", enabled: true }],
      setLocaleEnabled: async () => {
        toggled = true;
        return undefined as never;
      },
    } as unknown as GlossaAdminLocalesTab["client"];
    const el = document.createElement("glossa-admin-locales-tab") as GlossaAdminLocalesTab;
    el.client = fakeClient;
    el.slug = "demo";
    document.body.appendChild(el);
    await flush();
    await el.updateComplete;
    // First action button in the first row is the enable/disable
    // toggle (gl-button wraps a native button inside its shadow
    // root, so we drill through).
    const ghost = el.shadowRoot!.querySelector("tbody tr gl-button") as HTMLElement & {
      shadowRoot: ShadowRoot;
    };
    const btn = ghost.shadowRoot.querySelector("button") as HTMLButtonElement;
    btn.click();
    await flush();
    expect(toggled).toBe(true);
  });
});

describe("<glossa-admin-users-tab>", () => {
  it("renders the create-user form + user list", async () => {
    const fakeClient = {
      listUsers: async () => [
        { id: "u1", email: "felix@example.com", role: "admin", locales: [] },
      ],
    } as unknown as GlossaAdminUsersTab["client"];
    const el = document.createElement("glossa-admin-users-tab") as GlossaAdminUsersTab;
    el.client = fakeClient;
    document.body.appendChild(el);
    await flush();
    await el.updateComplete;
    expect(el.shadowRoot!.querySelector("form")).toBeTruthy();
    expect(el.shadowRoot!.querySelectorAll("tbody tr").length).toBe(1);
  });
});

describe("<glossa-admin-audit-tab>", () => {
  it("renders audit rows", async () => {
    const fakeClient = {
      audit: async () => [
        {
          id: 1,
          translationId: "tr-1",
          beforeValue: "old",
          afterValue: "new",
          changedBy: "u1",
          changedAt: "2026-05-23T10:00:00Z",
        },
      ],
    } as unknown as GlossaAdminAuditTab["client"];
    const el = document.createElement("glossa-admin-audit-tab") as GlossaAdminAuditTab;
    el.client = fakeClient;
    document.body.appendChild(el);
    await flush();
    await el.updateComplete;
    expect(el.shadowRoot!.querySelectorAll("tbody tr").length).toBe(1);
  });
});

describe("<glossa-admin-bulk-tab>", () => {
  it("renders the locale picker once locales load", async () => {
    const fakeClient = {
      listLocales: async () => [{ id: "l1", code: "de", label: "Deutsch", enabled: true }],
    } as unknown as GlossaAdminBulkTab["client"];
    const el = document.createElement("glossa-admin-bulk-tab") as GlossaAdminBulkTab;
    el.client = fakeClient;
    el.slug = "demo";
    document.body.appendChild(el);
    await flush();
    await el.updateComplete;
    // The picker is now a <gl-select> host whose internal
    // <select> lives behind a shadow root. The host alone is
    // proof the tab progressed past the loading state.
    expect(el.shadowRoot!.querySelector("gl-select")).toBeTruthy();
  });
});
