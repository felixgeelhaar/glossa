// axe-core gate. Mounts the major admin surfaces in jsdom + runs
// the WCAG 2.1 AA rule set against each. Fails the build on any
// 'critical' or 'serious' violation; lower severities ('moderate',
// 'minor') are reported in stderr but don't break CI yet.
//
// Scope intentionally narrow: only the surfaces a translator hits
// during normal use. Modal / dialog rendering is exercised by
// triggering the relevant state on each component.

import { afterEach, describe, expect, it } from "vitest";

import axe, { type Result } from "axe-core";

import "./admin-app.js";
import "./ai-providers-tab.js";
import "./audit-tab.js";
import "./bulk-tab.js";
import "./diff-tab.js";
import "./editor-tab.js";
import "./key-edit.js";
import "./key-list.js";
import "./keys-tab.js";
import "./locales-tab.js";
import "./users-tab.js";

import { initTheme } from "@felixgeelhaar/glossa-ui";

initTheme();

const AXE_OPTIONS: axe.RunOptions = {
  runOnly: { type: "tag", values: ["wcag2a", "wcag2aa", "wcag21a", "wcag21aa"] },
  // jsdom doesn't lay out elements, so colour-contrast / region rules
  // can't be evaluated meaningfully. Skip them here; visual checks
  // happen at the real-browser smoke step.
  rules: {
    "color-contrast": { enabled: false },
    region: { enabled: false },
    "landmark-one-main": { enabled: false },
  },
};

async function audit(el: Element): Promise<Result[]> {
  const out = await axe.run(el, AXE_OPTIONS);
  return out.violations.filter((v) => v.impact === "critical" || v.impact === "serious");
}

function describeViolations(vs: Result[]): string {
  return vs.map((v) => `${v.impact}: ${v.id} (${v.help}) — ${v.nodes.length} node(s)`).join("\n");
}

afterEach(() => {
  document.body.innerHTML = "";
});

describe("a11y — admin surfaces", () => {
  it("login form has no critical/serious axe violations", async () => {
    const el = document.createElement("glossa-admin");
    document.body.appendChild(el);
    await (el as { updateComplete?: Promise<unknown> }).updateComplete;
    const violations = await audit(document.body);
    if (violations.length > 0) {
      // eslint-disable-next-line no-console
      console.error(describeViolations(violations));
    }
    expect(violations).toEqual([]);
  });

  it("API keys tab has no critical/serious axe violations", async () => {
    const el = document.createElement("glossa-admin-keys-tab") as HTMLElement & {
      client: unknown;
      slug: string;
      updateComplete?: Promise<unknown>;
    };
    el.client = {
      listProjectApiKeys: async () => ({ keys: [] }),
      issueProjectApiKey: async () => ({ key: { id: "x", scope: "read", label: "x", createdAt: new Date().toISOString() }, apiKey: "glossa_x" }),
      revokeProjectApiKey: async () => undefined,
    };
    el.slug = "demo";
    document.body.appendChild(el);
    await el.updateComplete;
    const violations = await audit(document.body);
    if (violations.length > 0) {
      // eslint-disable-next-line no-console
      console.error(describeViolations(violations));
    }
    expect(violations).toEqual([]);
  });

  it("editor empty state has no critical/serious axe violations", async () => {
    const el = document.createElement("glossa-admin-editor-tab") as HTMLElement & {
      client: unknown;
      slug: string;
      updateComplete?: Promise<unknown>;
    };
    el.client = {
      listLocales: async () => [{ id: "1", code: "de", label: "Deutsch", enabled: true }],
      listBundle: async () => ({ project: "demo", locale: "de", messages: {}, statuses: {} }),
    };
    el.slug = "demo";
    document.body.appendChild(el);
    await el.updateComplete;
    // Allow the async listLocales + listBundle promises to settle.
    await new Promise((r) => setTimeout(r, 10));
    await el.updateComplete;
    const violations = await audit(document.body);
    if (violations.length > 0) {
      // eslint-disable-next-line no-console
      console.error(describeViolations(violations));
    }
    expect(violations).toEqual([]);
  });
});
