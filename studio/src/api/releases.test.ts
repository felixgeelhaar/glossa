import { afterEach, describe, expect, it, vi } from "vitest";
import { setCsrfToken } from "./client";
import { apiReleases } from "./releases";

const json = (status: number, body: unknown, headers: Record<string, string> = {}) =>
  new Response(status === 204 ? null : JSON.stringify(body), {
    status,
    headers: { "content-type": status >= 400 ? "application/problem+json" : "application/json", ...headers },
  });

function mockFetch(...responses: Response[]) {
  const fn = vi.fn<(r: Request) => Promise<Response>>();
  for (const r of responses) fn.mockResolvedValueOnce(r);
  vi.stubGlobal("fetch", fn);
  return fn;
}

const p = { tenant: "t", project: "p" };
const policy = { states: ["approved"], include_outdated: true };
const env = { name: "production", kind: "standard", policy, current_release_id: "r1", created_at: "2026-09-19T08:00:00Z", updated_at: "2026-09-19T08:00:00Z" };
const release = {
  id: "r1",
  version: 1,
  environment: "staging",
  policy,
  manifest_digest: "a".repeat(64),
  source_locale: "en",
  locales: [{ code: "en", direction: "ltr" }],
  counts: { messages: 3, artifacts: 1, bytes: 120, new_artifacts: 1, locales: { en: { messages: 3, outdated: 0 } } },
  author: "person:me",
  created_at: "2026-09-19T08:00:00Z",
};

afterEach(() => setCsrfToken(undefined));

describe("apiReleases", () => {
  it("publishes with an Idempotency-Key and the CSRF token", async () => {
    setCsrfToken("csrf-1");
    const fetch = mockFetch(json(201, release));
    const r = await apiReleases.publish(p, { environment: "staging", note: "Spring" }, "idem-1");
    expect(r.version).toBe(1);
    const req = fetch.mock.calls[0]![0];
    expect(req.method).toBe("POST");
    expect(new URL(req.url).pathname).toBe("/v1/tenants/t/projects/p/releases");
    expect(req.headers.get("Idempotency-Key")).toBe("idem-1");
    expect(req.headers.get("X-CSRF-Token")).toBe("csrf-1");
    expect(await req.json()).toEqual({ environment: "staging", note: "Spring" });
  });

  it("changes a policy with If-Match and returns the ETag on read", async () => {
    const fetch = mockFetch(json(200, env, { ETag: '"3"' }), json(200, env, { ETag: '"4"' }));
    const got = await apiReleases.environment(p, "production");
    expect(got.etag).toBe('"3"');
    await apiReleases.updatePolicy(p, "production", { states: ["approved"], include_outdated: false }, got.etag!);
    const patch = fetch.mock.calls[1]![0];
    expect(patch.method).toBe("PATCH");
    expect(patch.headers.get("If-Match")).toBe('"3"');
    expect(await patch.json()).toEqual({ policy: { states: ["approved"], include_outdated: false } });
  });

  it("rolls back to an explicit release and diffs against a base", async () => {
    const fetch = mockFetch(json(200, env), json(200, { release_id: "r2", base_release_id: "r1", locales: [] }));
    await apiReleases.rollback(p, "production", "r1");
    await apiReleases.diff(p, "r2", "r1");
    const [rb, diff] = fetch.mock.calls.map(([r]) => r);
    expect(new URL(rb!.url).pathname).toBe("/v1/tenants/t/projects/p/environments/production/rollbacks");
    expect(await rb!.json()).toEqual({ release_id: "r1" });
    expect(new URL(diff!.url).search).toBe("?base=r1");
  });

  it("maps release problems to their codes", async () => {
    mockFetch(json(409, { type: "urn:x", title: "Conflict", status: 409, code: "release_ineligible", detail: "no" }));
    await expect(apiReleases.promote(p, "production", "r1")).rejects.toMatchObject({ status: 409, code: "release_ineligible" });
  });

  it("rejects a response that breaks the contract", async () => {
    mockFetch(json(200, { ...release, manifest_digest: "not-a-digest" }));
    await expect(apiReleases.release(p, "r1")).rejects.toMatchObject({ code: "invalid_response" });
  });

  it("asks for a publish's dry run with the CSRF token", async () => {
    setCsrfToken("csrf-2");
    const fetch = mockFetch(
      json(200, {
        environment: "staging",
        policy,
        base_release_id: "r1",
        releasable: true,
        problems: [],
        manifest_digest: "b".repeat(64),
        source_locale: "en",
        locales: [{ code: "en", direction: "ltr" }],
        counts: release.counts,
        changes: [{ locale: "en", added: ["a"], changed: [], removed: [] }],
      }),
    );
    const pv = await apiReleases.previewPublish(p, "staging");
    expect(pv.changes?.[0]?.added).toEqual(["a"]);
    const req = fetch.mock.calls[0]![0];
    expect(req.method).toBe("POST");
    expect(new URL(req.url).pathname).toBe("/v1/tenants/t/projects/p/environments/staging/release-previews");
    expect(req.headers.get("X-CSRF-Token")).toBe("csrf-2");
  });

  it("revokes a delivery key", async () => {
    const fetch = mockFetch(json(204, null));
    await apiReleases.revokeDeliveryKey(p, "dk1");
    const req = fetch.mock.calls[0]![0];
    expect(req.method).toBe("DELETE");
    expect(new URL(req.url).pathname).toBe("/v1/tenants/t/projects/p/delivery-keys/dk1");
  });

  it("creates a key with a scope and changes an existing one's", async () => {
    const key = {
      id: "dk1",
      name: "previews",
      key: "glossa_pk_" + "a".repeat(32),
      scope: { environments: ["preview"], branches: true },
      created_by: "person:me",
      created_at: "2026-09-19T08:00:00Z",
    };
    const fetch = mockFetch(json(201, key), json(200, key));
    const scope = { environments: ["preview"], branches: true };
    const created = await apiReleases.createDeliveryKey(p, "previews", scope, "idem-2");
    expect(created.scope).toEqual(scope);
    const post = fetch.mock.calls[0]![0];
    expect(await post.json()).toEqual({ name: "previews", scope });

    await apiReleases.setDeliveryKeyScope(p, "dk1", scope);
    const put = fetch.mock.calls[1]![0];
    expect(put.method).toBe("PUT");
    expect(new URL(put.url).pathname).toBe("/v1/tenants/t/projects/p/delivery-keys/dk1/scope");
    expect(await put.json()).toEqual(scope);
  });
});
