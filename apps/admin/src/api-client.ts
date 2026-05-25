// Thin admin-side HTTP client over the Glossa REST surface. Wraps
// fetch with the JWT auth header + a tiny error type. Lives in
// apps/admin (not @felixgeelhaar/glossa-sdk) because the admin only ever talks
// to the admin endpoints — the SDK targets the public consumer
// surface and stays narrow.

export interface AuthState {
  token: string;
  expires: string;
  user: { id: string; email: string; role: "admin" | "translator"; locales: string[] };
  tenant: { id: string; slug: string; name: string };
}

export interface ProjectRow {
  id: string;
  slug: string;
  name: string;
  defaultLocale: string;
}

export interface LocaleRow {
  id: string;
  code: string;
  label: string;
  enabled: boolean;
}

export interface UserRow {
  id: string;
  email: string;
  role: "admin" | "translator";
  locales: string[];
}

export interface AuditRow {
  id: number;
  translationId: string | null;
  beforeValue: string;
  afterValue: string;
  changedBy: string | null;
  actorKind?: "user" | "ai" | "system";
  actorLabel?: string;
  changedAt: string;
}

export interface DiffRow {
  locale: string;
  label: string;
  total: number;
  pending: number;
  aiTranslated?: number;
  needsReview: number;
  approved: number;
}

export interface BundleResponse {
  project: string;
  locale: string;
  messages: Record<string, string>;
  statuses: Record<string, "pending" | "needs_review" | "approved" | "ai_translated">;
}

export class ApiError extends Error {
  public readonly status: number;
  public constructor(message: string, status: number) {
    super(message);
    this.name = "ApiError";
    this.status = status;
  }
}

export interface AdminClientConfig {
  apiUrl: string;
  token: string;
  fetch?: typeof fetch;
}

/** Build a JSON-aware fetch helper bound to a specific apiUrl + JWT. */
export function adminClient(cfg: AdminClientConfig) {
  const base = cfg.apiUrl.replace(/\/+$/, "") + "/api/v1/admin";
  const doFetch = cfg.fetch ?? fetch;

  async function req<T>(path: string, init: RequestInit = {}): Promise<T> {
    const headers = new Headers(init.headers);
    headers.set("Authorization", "Bearer " + cfg.token);
    if (init.body && !headers.has("Content-Type")) {
      headers.set("Content-Type", "application/json");
    }
    headers.set("Accept", "application/json");
    const res = await doFetch(base + path, { ...init, headers });
    if (res.status === 204) return undefined as unknown as T;
    const text = await res.text();
    if (!res.ok) {
      throw new ApiError(text || res.statusText, res.status);
    }
    return text ? (JSON.parse(text) as T) : (undefined as unknown as T);
  }

  return {
    me: () => req<{ id: string; email: string; role: string; locales: string[]; tenantId: string }>("/me"),

    listProjects: () => req<ProjectRow[]>("/projects"),
    createProject: (input: { tenantId: string; slug: string; name: string; defaultLocale?: string }) =>
      req<{ id: string; slug: string; name: string; apiKey: string }>("/projects", {
        method: "POST",
        body: JSON.stringify(input),
      }),
    listProjectApiKeys: (slug: string) =>
      req<{ keys: ProjectApiKeyRow[] }>(`/projects/${encodeURIComponent(slug)}/api-keys`),
    issueProjectApiKey: (slug: string, input: { scope: "read" | "write"; label: string }) =>
      req<{ key: ProjectApiKeyRow; apiKey: string }>(
        `/projects/${encodeURIComponent(slug)}/api-keys`,
        { method: "POST", body: JSON.stringify(input) },
      ),
    revokeProjectApiKey: (slug: string, id: string) =>
      req(`/projects/${encodeURIComponent(slug)}/api-keys/${id}`, { method: "DELETE" }),

    listLocales: (slug: string) => req<LocaleRow[]>(`/projects/${encodeURIComponent(slug)}/locales`),
    createLocale: (slug: string, input: { code: string; label: string }) =>
      req<LocaleRow>(`/projects/${encodeURIComponent(slug)}/locales`, {
        method: "POST",
        body: JSON.stringify(input),
      }),
    setLocaleEnabled: (slug: string, id: string, enabled: boolean) =>
      req(`/projects/${encodeURIComponent(slug)}/locales/${id}/enabled`, {
        method: "PATCH",
        body: JSON.stringify({ enabled }),
      }),
    deleteLocale: (slug: string, id: string) =>
      req(`/projects/${encodeURIComponent(slug)}/locales/${id}`, { method: "DELETE" }),

    listBundle: (slug: string, locale: string) =>
      req<BundleResponse>(
        `/projects/${encodeURIComponent(slug)}/locales/${encodeURIComponent(locale)}/messages`,
      ),
    patchTranslation: (slug: string, locale: string, key: string, body: { value: string; status?: string }) =>
      req<{ id: string; value: string; status: string }>(
        `/projects/${encodeURIComponent(slug)}/locales/${encodeURIComponent(locale)}/keys/${encodeURIComponent(key)}`,
        { method: "PATCH", body: JSON.stringify(body) },
      ),
    bulkImport: (slug: string, locale: string, messages: Record<string, string>, status?: string) =>
      req<{ applied: number; failed: number; results: Array<{ key: string; id?: string; error?: string }> }>(
        `/projects/${encodeURIComponent(slug)}/locales/${encodeURIComponent(locale)}/bulk`,
        { method: "POST", body: JSON.stringify({ messages, status }) },
      ),
    diff: (slug: string) => req<{ project: string; locales: DiffRow[] }>(`/projects/${encodeURIComponent(slug)}/diff`),

    listUsers: () => req<UserRow[]>("/users"),
    createUser: (input: { email: string; password: string; role: string; locales?: string[] }) =>
      req<UserRow>("/users", { method: "POST", body: JSON.stringify(input) }),
    updateUserLocales: (id: string, locales: string[]) =>
      req(`/users/${id}/locales`, { method: "PATCH", body: JSON.stringify({ locales }) }),
    deleteUser: (id: string) => req(`/users/${id}`, { method: "DELETE" }),

    audit: (limit = 200) => req<AuditRow[]>(`/audit?limit=${limit}`),

    tenantMetrics: () => req<{ firstEvents: TenantFirstEventRow[] }>("/metrics"),
    projectMetrics: (slug: string) =>
      req<{ project: string; events: ProjectMetricRow[] }>(
        `/projects/${encodeURIComponent(slug)}/metrics`,
      ),

    listAIProviders: () => req<{ providers: AIProviderRow[] }>("/ai-providers"),
    createAIProvider: (input: {
      kind: string;
      label: string;
      baseUrl?: string;
      model: string;
      apiKey: string;
      enabled?: boolean;
    }) => req<AIProviderRow>("/ai-providers", { method: "POST", body: JSON.stringify(input) }),
    updateAIProvider: (
      id: string,
      input: { label: string; baseUrl?: string; model: string; enabled: boolean; apiKey?: string },
    ) =>
      req(`/ai-providers/${id}`, { method: "PATCH", body: JSON.stringify(input) }),
    deleteAIProvider: (id: string) => req(`/ai-providers/${id}`, { method: "DELETE" }),
    testAIProvider: (id: string, source: string) =>
      req<{ ok: boolean; translation?: string; provider?: string; error?: string }>(
        `/ai-providers/${id}/test`,
        { method: "POST", body: JSON.stringify({ source }) },
      ),
  };
}

/** Analytics event kinds emitted server-side. */
export type AnalyticsKind =
  | "project_created"
  | "first_key_synced"
  | "first_translation_edited"
  | "first_consumer_request"
  | "first_ai_translation"
  | "translation_edited"
  | "consumer_request"
  | "ai_translation";

export interface ProjectMetricRow {
  kind: AnalyticsKind;
  firstAt: string;
  total: number;
}

export interface TenantFirstEventRow {
  projectId: string;
  kind: AnalyticsKind;
  firstAt: string;
}

/** Row shape returned by /admin/projects/:slug/api-keys. */
export interface ProjectApiKeyRow {
  id: string;
  scope: "read" | "write";
  label: string;
  createdAt: string;
  lastUsedAt?: string | null;
  revokedAt?: string | null;
}

/** Row shape returned by /admin/ai-providers. */
export interface AIProviderRow {
  id: string;
  kind: string;
  label: string;
  baseUrl: string;
  model: string;
  enabled: boolean;
  createdAt: string;
}

/** Tenant entry returned by /auth/discover. */
export interface TenantOption {
  slug: string;
  name: string;
}

/** POST /api/v1/auth/discover — returns tenants the email belongs to. */
export async function discoverTenants(apiUrl: string, email: string): Promise<TenantOption[]> {
  const base = apiUrl.replace(/\/+$/, "");
  const res = await fetch(base + "/api/v1/auth/discover", {
    method: "POST",
    headers: { "Content-Type": "application/json", Accept: "application/json" },
    body: JSON.stringify({ email }),
  });
  if (!res.ok) throw new ApiError(res.statusText, res.status);
  const body = (await res.json()) as { tenants: TenantOption[] };
  return body.tenants ?? [];
}

/** POST /api/v1/auth/login — exchange (tenant, email, password) for a JWT. */
export async function login(apiUrl: string, tenantSlug: string, email: string, password: string): Promise<AuthState> {
  const base = apiUrl.replace(/\/+$/, "");
  const res = await fetch(base + "/api/v1/auth/login", {
    method: "POST",
    headers: { "Content-Type": "application/json", Accept: "application/json" },
    body: JSON.stringify({ tenantSlug, email, password }),
  });
  if (!res.ok) {
    throw new ApiError(res.statusText, res.status);
  }
  return (await res.json()) as AuthState;
}
