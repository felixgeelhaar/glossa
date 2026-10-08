import type { ReviewState } from "../api/schemas";

/** Colour tone of a review state (pill-ok / pill-warn / …). */
export function stateTone(state: ReviewState): "ok" | "warn" | "err" | "neutral" {
  return { approved: "ok", needs_review: "warn", rejected: "err", draft: "neutral" }[state] as "ok" | "warn" | "err" | "neutral";
}

/**
 * Whether `self` wrote the current text: an author never approves or
 * rejects their own work (RFC 0006 §15 Q6). Imported text is written by
 * the importer, not authored, so it stays reviewable.
 */
export function isOwnText(translation: { author: string; origin: string } | null | undefined, selfId: string | undefined): boolean {
  if (!translation || !selfId || translation.origin === "import") return false;
  return translation.author === `person:${selfId}`;
}

/** Review actions a person may take on a translation in `state`; `ownText` withholds approve and reject. */
export function reviewActions(state: ReviewState | undefined, canWrite: boolean, canReview: boolean, ownText = false): Array<"approve" | "reject" | "request"> {
  if (!state) return [];
  const out: Array<"approve" | "reject" | "request"> = [];
  if (canReview && !ownText && state !== "approved") out.push("approve");
  if (canReview && !ownText && state !== "rejected") out.push("reject");
  if (canWrite && (state === "draft" || state === "rejected")) out.push("request");
  return out;
}
