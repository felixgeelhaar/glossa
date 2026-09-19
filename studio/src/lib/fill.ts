/** Progress of an AI fill from its jobs' states. */
import type { AIJob } from "../api/intelligence-schemas";

/** How often a running fill's jobs are polled. */
export const FILL_POLL_MS = 1000;

export interface FillProgress {
  total: number;
  queued: number;
  running: number;
  /** Queued or running: not settled yet. */
  active: number;
  succeeded: number;
  skipped: number;
  /** Failed for good, dead or cancelled. */
  failed: number;
  /** Why jobs failed, most common first. */
  failures: Array<[code: string, count: number]>;
}

export function fillProgress(jobs: readonly Pick<AIJob, "state" | "failure_code">[]): FillProgress {
  const p: FillProgress = { total: jobs.length, queued: 0, running: 0, active: 0, succeeded: 0, skipped: 0, failed: 0, failures: [] };
  const codes = new Map<string, number>();
  for (const j of jobs) {
    switch (j.state) {
      case "queued":
        p.queued++;
        break;
      case "running":
        p.running++;
        break;
      case "succeeded":
        p.succeeded++;
        break;
      case "skipped":
        p.skipped++;
        break;
      default: {
        p.failed++;
        const code = j.state === "failed" ? (j.failure_code ?? "unknown") : j.state;
        codes.set(code, (codes.get(code) ?? 0) + 1);
      }
    }
  }
  p.active = p.queued + p.running;
  p.failures = [...codes].sort((a, b) => b[1] - a[1] || a[0].localeCompare(b[0]));
  return p;
}
