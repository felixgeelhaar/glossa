import type { ReviewState } from "../api/schemas";

/** Colour tone of a review state (pill-ok / pill-warn / …). */
export function stateTone(state: ReviewState): "ok" | "warn" | "err" | "neutral" {
  return { approved: "ok", needs_review: "warn", rejected: "err", draft: "neutral" }[state] as "ok" | "warn" | "err" | "neutral";
}

/** Review actions a person may take on a translation in `state`. */
export function reviewActions(state: ReviewState | undefined, canWrite: boolean, canReview: boolean): Array<"approve" | "reject" | "request"> {
  if (!state) return [];
  const out: Array<"approve" | "reject" | "request"> = [];
  if (canReview && state !== "approved") out.push("approve");
  if (canReview && state !== "rejected") out.push("reject");
  if (canWrite && (state === "draft" || state === "rejected")) out.push("request");
  return out;
}
