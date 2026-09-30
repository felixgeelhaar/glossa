/**
 * The quality summary port (RFC 0005 §8): the seven numbers for one
 * project, per project and per locale, from one cached endpoint.
 *
 * `apiQualitySummary` implements it with the response checked by zod,
 * as every other port here does. It does *not* go through the generated
 * client, because the operation is a sibling slice of wave 5 and is not
 * in platform/api/openapi.yaml yet; this slice consumes the API and does
 * not change it (RFC 0005 §13). The request below keeps the session
 * conventions of ./client.ts — same-origin cookie, a 401 outside the
 * auth endpoints means the session is gone — and should be replaced by
 * `client.GET(SUMMARY_PATH, …)` the moment the operation lands, with a
 * contract assertion added to ./quality-summary-schemas.ts.
 *
 * A project whose server does not answer this yet is *not* a healthy
 * project: `unavailable()` says so, and every screen renders it as "not
 * measured" rather than as zero.
 */
import { inject, type InjectionKey } from "vue";
import { reportUnauthenticated } from "./client";
import { isApiError, read, type RawResult, type Versioned } from "./errors";
import { QualitySummary } from "./quality-summary-schemas";
import type { ProjectRef } from "./releases";

export const SUMMARY_PATH = "/v1/tenants/{tenant}/projects/{project}/quality-summary" as const;

export interface QualitySummaryPort {
  /** The seven numbers for this project, per project and per locale. */
  summary(p: ProjectRef, signal?: AbortSignal): Promise<QualitySummary>;
}

const path = (p: ProjectRef): string =>
  SUMMARY_PATH.replace("{tenant}", encodeURIComponent(p.tenant)).replace("{project}", encodeURIComponent(p.project));

/** One GET with ./client.ts's session conventions, shaped like what openapi-fetch resolves with. */
async function get(url: string, signal?: AbortSignal): Promise<RawResult> {
  const base = globalThis.location?.origin ?? "";
  const request = new Request(new URL(url, base || "http://localhost"), {
    method: "GET",
    credentials: "same-origin",
    headers: { Accept: "application/json" },
    ...(signal ? { signal } : {}),
  });
  const response = await globalThis.fetch(request);
  if (response.status === 401) reportUnauthenticated(new URL(request.url).pathname);
  const body = response.status === 204 ? undefined : await response.json().catch(() => undefined);
  return response.ok ? { data: body, response } : { error: body, response };
}

const value = async <T>(p: Promise<Versioned<T>>): Promise<T> => (await p).value;

export const apiQualitySummary: QualitySummaryPort = {
  summary: (p, signal) => value(read(get(path(p), signal), QualitySummary)),
};

export const QUALITY_SUMMARY: InjectionKey<QualitySummaryPort> = Symbol("quality-summary");

/** The provided port, else the API adapter. */
export function useQualitySummary(): QualitySummaryPort {
  return inject(QUALITY_SUMMARY, apiQualitySummary);
}

/**
 * Whether a failed summary read means "this server does not report the
 * numbers" rather than "something went wrong". A 404 or a 501 is the
 * former — the endpoint is not deployed here — and the screens answer it
 * with "not measured", quietly. Anything else is a real failure and is
 * shown.
 */
export function notReported(e: unknown): boolean {
  if (!isApiError(e)) return false;
  return e.status === 404 || e.status === 501 || e.code === "not_found" || e.code === "unexpected_response";
}
