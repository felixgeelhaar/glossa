import { describe, expect, it } from "vitest";
import { reviewActions, stateTone } from "./review";

describe("stateTone", () => {
  it("maps review states to tones", () => {
    expect(stateTone("approved")).toBe("ok");
    expect(stateTone("needs_review")).toBe("warn");
    expect(stateTone("rejected")).toBe("err");
    expect(stateTone("draft")).toBe("neutral");
  });
});

describe("reviewActions", () => {
  it("offers review only to reviewers", () => {
    expect(reviewActions("needs_review", true, false)).toEqual([]);
    expect(reviewActions("needs_review", true, true)).toEqual(["approve", "reject"]);
    expect(reviewActions("approved", true, true)).toEqual(["reject"]);
    expect(reviewActions("rejected", true, true)).toEqual(["approve", "request"]);
  });
  it("lets writers send drafts to review", () => {
    expect(reviewActions("draft", true, false)).toEqual(["request"]);
    expect(reviewActions(undefined, true, true)).toEqual([]);
  });
});
