import { flushPromises, type VueWrapper } from "@vue/test-utils";
import { afterEach, describe, expect, it } from "vitest";
import { createFakeIntelligence, suggestion, type FakeIntelligence } from "../../test/fake-intelligence";
import { locale, mountProjectScreen, type ScreenOptions } from "../../test/project";
import ReviewQueueView from "./ReviewQueueView.vue";

let wrapper: VueWrapper | undefined;
afterEach(() => {
  wrapper?.unmount();
  wrapper = undefined;
});

const locales = [locale("en", true), locale("de"), locale("fr")];
const message = (key: string) => ({
  id: key,
  key,
  namespace: "default",
  description: "",
  state: "active",
  source: { text: `source of ${key}`, syntax: "mf1", model: { type: "message" }, arguments: [], markup: [] },
  source_revision: 1,
  created_at: "t",
  updated_at: "t",
});

function queue(): FakeIntelligence {
  const i = createFakeIntelligence();
  i.state.suggestions.push(
    suggestion({ id: "ok", message_key: "billing.invoice", score: 0.86, action: "approve_recommended", message: "Rechnung herunterladen" }),
    suggestion({ id: "risky", message_key: "workspace.delete", score: 0.23, risk_tags: ["term_forbidden"], message: "Diesen Workspace löschen" }),
    suggestion({ id: "legal", message_key: "terms.accept", score: 0.49, risk_tags: ["legal"] }),
    suggestion({ id: "fr", message_key: "menu.home", locale: "fr", score: 0.9, action: "approve_recommended", message: "Accueil" }),
  );
  return i;
}

async function screen(i: FakeIntelligence, options: Partial<ScreenOptions> = {}) {
  wrapper = await mountProjectScreen(ReviewQueueView, {
    intelligence: i,
    locales,
    roles: ["owner"],
    path: "/t/t/p/p/review",
    fetch: (path) => message(path.split("/").at(-1)!),
    ...options,
  });
  return wrapper;
}

const press = async (key: string, init: KeyboardEventInit = {}, target: EventTarget = window) => {
  (target as HTMLElement).dispatchEvent(new KeyboardEvent("keydown", { key, bubbles: true, cancelable: true, ...init }));
  await flushPromises();
};
const options = (w: VueWrapper) => w.findAll("[role=option]");

describe("ReviewQueueView", () => {
  it("lists pending suggestions riskiest first, with the focused one's source", async () => {
    const w = await screen(queue());
    expect(options(w).map((o) => o.get(".key").text())).toEqual(["workspace.delete", "terms.accept", "billing.invoice", "menu.home"]);
    expect(options(w)[0]!.text()).toContain("Low confidence · 0.23");
    expect(options(w)[0]!.text()).toContain("Forbidden term");
    expect(options(w)[0]!.attributes("aria-selected")).toBe("true");
    expect(w.get("[data-testid=review-source]").text()).toBe("source of workspace.delete");
  });

  it("triages from the keyboard: j/k move, a accepts, r rejects, e edits and Mod+Enter accepts the edit", async () => {
    const i = queue();
    const w = await screen(i);
    await press("j");
    expect(options(w)[1]!.attributes("aria-selected")).toBe("true");
    await press("a");
    expect(i.calls).toContainEqual(["accept", "legal", undefined]);
    expect(w.get("[data-testid=review-status]").text()).toBe("Accepted terms.accept (de).");
    expect(options(w)).toHaveLength(3);
    // The next item takes the decided one's place.
    expect(options(w)[1]!.attributes("aria-selected")).toBe("true");
    await press("k");
    await press("e");
    const edit = w.get("[data-testid=review-edit]");
    expect(document.activeElement).toBe(edit.element);
    // Single keys type while editing.
    await press("a", {}, edit.element);
    expect(i.calls.filter((c) => c[0] === "accept")).toHaveLength(1);
    await edit.setValue("Diesen Arbeitsbereich löschen");
    await press("Enter", { metaKey: true }, edit.element);
    expect(i.calls).toContainEqual(["accept", "risky", { text: "Diesen Arbeitsbereich löschen", syntax: "mf2" }]);
    expect(w.get("[data-testid=review-status]").text()).toBe("Accepted your edit of workspace.delete (de).");
    await press("r");
    expect(i.calls).toContainEqual(["reject", "ok", undefined]);
    expect(options(w).map((o) => o.get(".key").text())).toEqual(["menu.home"]);
  });

  it("Esc cancels an edit without deciding", async () => {
    const i = queue();
    const w = await screen(i);
    await press("e");
    await press("Escape", {}, w.get("[data-testid=review-edit]").element);
    expect(w.find("[data-testid=review-edit]").exists()).toBe(false);
    expect(i.calls.filter((c) => c[0] === "accept" || c[0] === "reject")).toEqual([]);
  });

  it("batch-accepts only what the policy recommends approving", async () => {
    const i = queue();
    const w = await screen(i);
    const batch = w.get("[data-testid=batch-accept]");
    expect(batch.text()).toBe("Accept 2 recommended");
    await batch.trigger("click");
    await flushPromises();
    const confirm = w.get("dialog[open]").findAll("button").find((b) => b.text() === "Accept 2")!;
    await confirm.trigger("click");
    await flushPromises();
    expect(i.calls.filter((c) => c[0] === "accept").map((c) => c[1])).toEqual(["ok", "fr"]);
    expect(w.get("[data-testid=review-status]").text()).toBe("Accepted 2.");
    expect(options(w).map((o) => o.get(".key").text())).toEqual(["workspace.delete", "terms.accept"]);
  });

  it("narrows a scoped translator's queue to their locales", async () => {
    const i = queue();
    const w = await screen(i, { roles: ["translator"], memberLocales: ["de"] });
    expect(i.calls.find((c) => c[0] === "reviewQueue")![1]).toEqual(["de"]);
    expect(options(w).map((o) => o.text())).not.toContain("menu.home");
    expect(w.findAll("#rq-locale option").map((o) => o.text())).toEqual(["All your locales", "de"]);
  });

  it("says when there's nothing to review", async () => {
    const w = await screen(createFakeIntelligence());
    expect(w.get("[data-testid=review-empty]").text()).toContain("Nothing to review.");
    expect(w.get("[data-testid=batch-accept]").attributes("disabled")).toBeDefined();
  });
});
