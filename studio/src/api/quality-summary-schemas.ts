/**
 * zod schemas for the quality summary (RFC 0005 §8): the seven numbers,
 * per project and per locale, behind one endpoint.
 *
 * Two shape rules run through all of this, and every screen that reads
 * it depends on them:
 *
 * - **Absent is not zero.** A number that was never measured is left
 *   out, never sent as `0`. `findings` absent means nothing has been
 *   checked; `findings.errors === 0` means a run looked and found none.
 *   The difference is the whole point of a health surface, so it is in
 *   the wire shape rather than in a flag beside it.
 * - **A layer names its availability per locale** (intent §41). A layer
 *   that cannot run for a locale is `available: false` with the reason,
 *   so it can never be drawn as a layer that ran and found nothing.
 *
 * These are not yet in platform/api/openapi.yaml — the summary endpoint
 * is a sibling slice of wave 5 (RFC 0005 §13) and this one consumes it
 * without touching the spec. There is therefore no compile-time
 * contract assertion here as in ./quality-schemas.ts; add one (and drop
 * the hand-rolled request in ./quality-summary.ts for the generated
 * client) as soon as the operation lands.
 */
import { z } from "zod";
import { FindingLayer } from "./quality-schemas";

const timestamp = z.string().min(1);
const count = z.number().int().min(0);
const share = z.number().min(0).max(1);

export const SUMMARY_SCHEMA = "glossa.quality-summary/v1";

/** Coverage (§8 row 1). Absent on a locale with no active messages to cover. */
export const SummaryCoverage = z.object({
  messages: count,
  translated: count,
  outdated: count,
  missing: count,
});

/** Outstanding findings (§8 row 2). Absent means nothing has ever been checked. */
export const SummaryFindings = z.object({
  errors: count,
  warnings: count,
  waived: count,
});

/** AI decisions (§8 row 3). Absent means nobody has accepted or rejected a suggestion yet. */
export const SummaryAi = z.object({
  decisions: count,
  acceptance_rate: share,
  mean_edit_distance: z.number().min(0),
});

/** A p50/p90 pair in seconds, with the sample it was taken over. Absent means there was nothing to measure. */
export const SummaryPercentiles = z.object({
  p50_seconds: z.number().min(0),
  p90_seconds: z.number().min(0),
  samples: count,
});

/**
 * The review queue (§8 row 4). `depth: 0` is a measured empty queue;
 * `age` absent with a depth above zero would be odd, but an empty queue
 * legitimately has no age to report.
 */
export const SummaryQueue = z.object({
  depth: count,
  age: SummaryPercentiles.optional(),
});

/** Context coverage (§8 row 5). Absent means no build has ever been uploaded, so nothing is known. */
export const SummaryContext = z.object({
  active_messages: count,
  with_usage: count,
  with_region: count,
});

/** Check health (§8 row 7): PR-check pass rate and time to a conclusion. Project-wide; checks are not per locale. */
export const SummaryChecks = z.object({
  runs: count,
  pass_rate: share,
  median_seconds: z.number().min(0).optional(),
});

/** Why a layer cannot run for a locale. Rendered as words, never as a colour alone. */
export const LayerUnavailable = z.enum([
  /** The layer has no rules or data for this language at all (intent §41). */
  "unsupported_locale",
  /** It could run, but nothing is set up yet: no termbase, no style guide, no policy. */
  "not_configured",
  /** It needs evidence this locale has none of: no capture, no usage. */
  "no_evidence",
]);

/**
 * One layer, for one locale. `available` false is the case this whole
 * slice exists for: it must never be drawn like a clean layer.
 * `checked` false with `available` true is the third state — it could
 * run, and this run did not.
 */
export const SummaryLayer = z.object({
  layer: FindingLayer,
  available: z.boolean(),
  /** Present exactly when `available` is false. */
  unavailable: LayerUnavailable.optional(),
  /** The newest run computed this layer for this locale. */
  checked: z.boolean(),
  /** Absent unless `checked`: a layer nobody ran has no counts, not zero counts. */
  findings: SummaryFindings.optional(),
});

export const LocaleHealth = z.object({
  code: z.string().min(1),
  direction: z.enum(["ltr", "rtl"]),
  is_source: z.boolean(),
  coverage: SummaryCoverage.optional(),
  findings: SummaryFindings.optional(),
  ai: SummaryAi.optional(),
  queue: SummaryQueue.optional(),
  context: SummaryContext.optional(),
  lead_time: SummaryPercentiles.optional(),
  /** Every layer RFC 0005 §3 defines, each saying whether it can run here. */
  layers: z.array(SummaryLayer),
});

export const ProjectHealth = z.object({
  coverage: SummaryCoverage.optional(),
  findings: SummaryFindings.optional(),
  ai: SummaryAi.optional(),
  queue: SummaryQueue.optional(),
  context: SummaryContext.optional(),
  lead_time: SummaryPercentiles.optional(),
  checks: SummaryChecks.optional(),
  /** The run the finding counts were taken from; absent when nothing has ever been checked. */
  run: z
    .object({
      id: z.string().min(1),
      ref: z.string(),
      policy_version: z.number().int().min(0),
      started_at: timestamp,
    })
    .optional(),
});

export const QualitySummary = z.object({
  schema: z.literal(SUMMARY_SCHEMA),
  /** When the cached summary was computed (§8: cached 60 s). */
  computed_at: timestamp,
  project: ProjectHealth,
  locales: z.array(LocaleHealth),
});

export type SummaryCoverage = z.infer<typeof SummaryCoverage>;
export type SummaryFindings = z.infer<typeof SummaryFindings>;
export type SummaryAi = z.infer<typeof SummaryAi>;
export type SummaryPercentiles = z.infer<typeof SummaryPercentiles>;
export type SummaryQueue = z.infer<typeof SummaryQueue>;
export type SummaryContext = z.infer<typeof SummaryContext>;
export type SummaryChecks = z.infer<typeof SummaryChecks>;
export type LayerUnavailable = z.infer<typeof LayerUnavailable>;
export type SummaryLayer = z.infer<typeof SummaryLayer>;
export type LocaleHealth = z.infer<typeof LocaleHealth>;
export type ProjectHealth = z.infer<typeof ProjectHealth>;
export type QualitySummary = z.infer<typeof QualitySummary>;
