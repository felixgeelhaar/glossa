// Wire types. Mirror the Go side's JSON shapes in
// apps/api/internal/interfaces/httpgin and the OpenAPI spec at
// api/openapi.yaml. Kept in this file so the rest of the SDK
// imports a single source of truth.

export type TranslationStatus = "pending" | "needs_review" | "approved";

/** GET /api/v1/projects/{slug}/locales/{locale}/messages response. */
export interface Bundle {
  project: string;
  locale: string;
  messages: Record<string, string>;
  statuses: Record<string, TranslationStatus>;
}

/** SSE `translation.updated` event payload. */
export interface TranslationUpdatedEvent {
  type: "translation.updated";
  project: string;
  locale: string;
  key: string;
  value: string;
  status: TranslationStatus;
}

/** One row of a `scan` batch upsert. */
export interface ScanInput {
  name: string;
  description?: string;
}

/** Per-row outcome for a `scan` batch. */
export interface ScanResult {
  name: string;
  id?: string;
  description?: string;
  error?: string;
}

/** POST /api/v1/projects/{slug}/keys:scan response shape. */
export interface ScanResponse {
  results: ScanResult[];
}

/** Configuration for [[createClient]]. */
export interface ClientConfig {
  /** Project slug — appears in every URL path. */
  project: string;
  /** API key. Sent as `Authorization: Bearer <apiKey>`. */
  apiKey: string;
  /** Base URL, e.g. `https://glossa.example.com`. No trailing slash. */
  apiUrl: string;
  /**
   * Override the fetch implementation. Lets tests inject a mock
   * without monkey-patching globalThis.fetch. Defaults to the
   * platform `fetch`.
   */
  fetch?: typeof fetch;
}
