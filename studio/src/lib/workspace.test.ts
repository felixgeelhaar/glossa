import { describe, expect, it } from "vitest";
import { matches, namespacesOf, statusOf } from "./workspace";

describe("statusOf", () => {
  const missing = new Set(["a"]);
  const outdated = new Set(["b"]);
  it("derives the row status", () => {
    expect(statusOf("a", missing, outdated)).toBe("missing");
    expect(statusOf("b", missing, outdated)).toBe("outdated");
    expect(statusOf("c", missing, outdated)).toBe("translated");
    expect(statusOf("c", undefined, outdated)).toBe("unknown");
  });
});

describe("matches", () => {
  const m = { key: "checkout.pay", description: "Button on the payment step", source: { text: "Pay {amount}" } } as Parameters<typeof matches>[0];
  it.each([
    ["", true],
    ["CHECKOUT", true],
    ["pay {", true],
    ["payment step", true],
    ["refund", false],
  ])("%j → %s", (q, want) => {
    expect(matches(m, q)).toBe(want);
  });
});

describe("namespacesOf", () => {
  it("dedupes and puts default first", () => {
    expect(namespacesOf([{ namespace: "emails" }, { namespace: "default" }, { namespace: "admin" }, { namespace: "emails" }], ["web"])).toEqual([
      "default",
      "admin",
      "emails",
      "web",
    ]);
  });
});
