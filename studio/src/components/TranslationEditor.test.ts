import { flushPromises, mount } from "@vue/test-utils";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "../api/errors";
import type { Message, Project, ProjectLocale, Translation } from "../api/schemas";
import { grantFor } from "../session/permissions";
import TranslationEditor from "./TranslationEditor.vue";

const api = vi.hoisted(() => ({
  get: vi.fn(),
  put: vi.fn(),
  review: vi.fn(),
  revisions: vi.fn(),
  sourceRevisions: vi.fn(),
}));
vi.mock("../api/endpoints", () => ({
  translations: { get: api.get, put: api.put, review: api.review, revisions: api.revisions },
  messages: { sourceRevisions: api.sourceRevisions },
}));

const now = "2026-09-19T12:00:00Z";
const project: Project = {
  id: "p",
  slug: "demo",
  name: "Demo",
  source_locale: "en",
  settings: { default_syntax: "mf1", review_required: true },
  created_at: now,
  updated_at: now,
};
const message: Message = {
  id: "m1",
  key: "greeting",
  namespace: "default",
  description: "Dashboard welcome",
  state: "active",
  source: {
    text: "Hello, {name}!",
    syntax: "mf1",
    model: { type: "message", declarations: [], pattern: ["Hello, ", { type: "expression", arg: { type: "variable", name: "name" } }, "!"] },
    arguments: [{ name: "name", type: "string" }],
    markup: [],
  },
  source_revision: 2,
  created_at: now,
  updated_at: now,
};
const de: ProjectLocale = { code: "de", direction: "ltr", is_source: false, created_at: now };
const en: ProjectLocale = { code: "en", direction: "ltr", is_source: true, created_at: now };

const translation = (over: Partial<Translation> = {}): Translation => ({
  id: "t1",
  message_id: "m1",
  locale: "de",
  text: "Hallo, {name}!",
  syntax: "mf1",
  model: { type: "message", declarations: [], pattern: ["Hallo, ", { type: "expression", arg: { type: "variable", name: "name" } }, "!"] },
  state: "needs_review",
  origin: "human",
  author: "person:me",
  source_revision: 2,
  current_source_revision: 2,
  outdated: false,
  warnings: [],
  revision: 1,
  created_at: now,
  updated_at: now,
  ...over,
});

const mountEditor = (roles: Array<"owner" | "translator"> = ["owner"], locales: string[] = []) =>
  mount(TranslationEditor, {
    props: { tenant: "t", project, message, locale: de, source: en, grant: grantFor({ roles, locales }) },
    attachTo: document.body,
  });

beforeEach(() => {
  for (const f of Object.values(api)) f.mockReset();
});

describe("TranslationEditor", () => {
  it("shows the source, its arguments and the developers' context", async () => {
    api.get.mockResolvedValue(null);
    const w = mountEditor();
    await flushPromises();
    expect(w.text()).toContain("Dashboard welcome");
    expect(w.get(".source-text").text()).toBe("Hello, {name}!");
    expect(w.get(".chips").text()).toContain("$name");
    const area = w.get("textarea");
    expect(area.attributes()).toMatchObject({ lang: "de", dir: "ltr" });
    w.unmount();
  });

  it("shows structural QA findings inline and marks the field invalid", async () => {
    api.get.mockResolvedValue(null);
    const findings = [
      { code: "missing-argument", severity: "error" as const, subject: "name", message: "the translation does not use {$name}" },
      { code: "extra-argument", severity: "error" as const, subject: "nam", message: "the translation uses {$nam}" },
    ];
    api.put.mockRejectedValue(
      new ApiError(422, "structural_qa_failed", "structurally incompatible", { type: "x", title: "t", status: 422, code: "structural_qa_failed", findings }),
    );
    const w = mountEditor();
    await flushPromises();
    await w.get("textarea").setValue("Hallo, {nam}!");
    await (w.vm as unknown as { save: (a?: boolean) => Promise<void> }).save();
    await flushPromises();
    const qa = w.get("[data-testid=qa-findings]");
    expect(qa.attributes("role")).toBe("alert");
    expect(qa.text()).toContain("missing-argument");
    expect(qa.text()).toContain("extra-argument");
    expect(w.get("textarea").attributes("aria-invalid")).toBe("true");
    expect(w.get("textarea").attributes("aria-describedby")).toContain("qa-errors");
    w.unmount();
  });

  it("saves with the ETag, and save+approve asks for approval", async () => {
    api.get.mockResolvedValue({ value: translation(), etag: '"e1"' });
    api.put.mockResolvedValue({ value: translation({ text: "Hallo {name}!", state: "approved", revision: 2 }), etag: '"e2"' });
    const w = mountEditor();
    await flushPromises();
    await w.get("textarea").setValue("Hallo {name}!");
    await (w.vm as unknown as { save: (a?: boolean) => Promise<void> }).save(true);
    await flushPromises();
    expect(api.put).toHaveBeenCalledWith(
      { tenant: "t", project: "p", message: "greeting", locale: "de" },
      { text: "Hallo {name}!", syntax: "mf1", state: "approved" },
      '"e1"',
    );
    expect(w.get("[data-testid=editor-status]").text()).toBe("Approved.");
    expect(w.get("[data-testid=translation-state]").text()).toBe("Approved");
    expect(w.emitted("changed")).toHaveLength(1);
    w.unmount();
  });

  it("approves an unchanged translation through the review endpoint", async () => {
    api.get.mockResolvedValue({ value: translation(), etag: '"e1"' });
    api.review.mockResolvedValue({ value: translation({ state: "approved" }), etag: '"e2"' });
    const w = mountEditor();
    await flushPromises();
    await (w.vm as unknown as { save: (a?: boolean) => Promise<void> }).save(true);
    await flushPromises();
    expect(api.put).not.toHaveBeenCalled();
    expect(api.review).toHaveBeenCalledWith(expect.anything(), "approved", '"e1"');
    w.unmount();
  });

  it("hides review actions from translators and keeps them in their locales", async () => {
    api.get.mockResolvedValue({ value: translation(), etag: '"e1"' });
    const w = mountEditor(["translator"], ["fr"]);
    await flushPromises();
    expect(w.find("textarea").attributes("readonly")).toBeDefined();
    expect(w.findAll("button").map((b) => b.text())).not.toContain("Approve");
    expect(w.text()).toContain("You can't write de translations");
    w.unmount();
  });

  it("flags an outdated translation and diffs the source it was made against", async () => {
    api.get.mockResolvedValue({ value: translation({ outdated: true, source_revision: 1 }), etag: '"e1"' });
    api.sourceRevisions.mockResolvedValue([
      { revision: 2, text: "Hello, {name}!", syntax: "mf1", model: message.source.model, author: "person:x", created_at: now },
      { revision: 1, text: "Hi {name}!", syntax: "mf1", model: message.source.model, author: "person:x", created_at: now },
    ]);
    const w = mountEditor();
    await flushPromises();
    expect(w.text()).toContain("Translated against source revision 1; the source is now at revision 2.");
    await w.findAll("button").find((b) => b.text() === "Show source changes")!.trigger("click");
    await flushPromises();
    expect(w.find("del").text()).toBe("Hi");
    expect(w.find("ins").text()).toBe("Hello,");
    w.unmount();
  });
});
