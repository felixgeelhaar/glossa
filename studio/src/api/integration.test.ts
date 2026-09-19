import { afterEach, describe, expect, it, vi } from "vitest";
import { onSessionExpired, setCsrfToken } from "./client";
import { ApiError } from "./errors";
import { apiIntegration, dispositionName } from "./integration";
import type { ExportJob, ImportJob } from "./integration-schemas";

const NOW = "2026-09-19T08:00:00Z";
const counts = { created: 0, updated: 0, unchanged: 0, conflict: 0, invalid: 0 };
const importJob: ImportJob = {
  id: "j1",
  project_id: "p",
  kind: "catalog",
  format: "xliff",
  mode: "dry_run",
  options: {},
  state: "awaiting_upload",
  file_name: "de.xlf",
  upload_url: "/v1/tenants/t/import-jobs/j1/file",
  summary: { ...counts, by_kind: {} },
  total_items: 0,
  processed_items: 0,
  cancel_requested: false,
  attempts: 0,
  created_by: "person:me",
  created_at: NOW,
  updated_at: NOW,
  expires_at: NOW,
};
const exportJob: ExportJob = {
  id: "e1",
  project_id: "p",
  kind: "catalog",
  format: "json",
  options: { locales: ["de", "en"] },
  state: "succeeded",
  file_name: "demo.json.zip",
  download_url: "/v1/tenants/t/export-jobs/e1/file",
  written: 4,
  cancel_requested: false,
  attempts: 1,
  created_by: "person:me",
  created_at: NOW,
  updated_at: NOW,
  expires_at: NOW,
};

/** Just enough XMLHttpRequest: records what was sent, answers with `respond`. */
class FakeXHR {
  static last: FakeXHR | undefined;
  static respond: (x: FakeXHR) => { status: number; body: string } = () => ({ status: 200, body: "{}" });
  method = "";
  url = "";
  headers: Record<string, string> = {};
  withCredentials = false;
  status = 0;
  responseText = "";
  body: unknown;
  private listeners = new Map<string, Array<(e: unknown) => void>>();
  upload = {
    progress: [] as Array<(e: ProgressEvent) => void>,
    addEventListener: (_t: string, f: (e: ProgressEvent) => void) => this.upload.progress.push(f),
  };
  open(method: string, url: string) {
    this.method = method;
    this.url = url;
  }
  setRequestHeader(k: string, v: string) {
    this.headers[k] = v;
  }
  addEventListener(t: string, f: (e: unknown) => void) {
    this.listeners.set(t, [...(this.listeners.get(t) ?? []), f]);
  }
  abort() {
    for (const f of this.listeners.get("abort") ?? []) f({});
  }
  send(body: unknown) {
    FakeXHR.last = this;
    this.body = body;
    queueMicrotask(() => {
      for (const f of this.upload.progress) f({ loaded: 5, total: 10, lengthComputable: true } as ProgressEvent);
      const r = FakeXHR.respond(this);
      this.status = r.status;
      this.responseText = r.body;
      for (const f of this.listeners.get("load") ?? []) f({});
    });
  }
}

afterEach(() => {
  setCsrfToken(undefined);
  FakeXHR.last = undefined;
});

describe("dispositionName", () => {
  it("reads plain, quoted and RFC 5987 names, never a path", () => {
    expect(dispositionName("attachment; filename=web.de.json")).toBe("web.de.json");
    expect(dispositionName('attachment; filename="shop.json.zip"')).toBe("shop.json.zip");
    expect(dispositionName("attachment; filename*=UTF-8''k%C3%BCche.de.xlf")).toBe("küche.de.xlf");
    expect(dispositionName('attachment; filename="../../etc/passwd"')).toBe("passwd");
    expect(dispositionName(null)).toBeUndefined();
  });
});

describe("upload", () => {
  it("PUTs the file with the CSRF token and reports progress", async () => {
    vi.stubGlobal("XMLHttpRequest", FakeXHR);
    FakeXHR.respond = () => ({ status: 200, body: JSON.stringify({ ...importJob, upload_url: undefined, state: "queued" }) });
    setCsrfToken("csrf-1");
    const progress: number[] = [];
    const file = new Blob(["<xliff/>"]);
    const job = await apiIntegration.upload("t", importJob, file, (l, t) => progress.push(l / t));
    expect(job.state).toBe("queued");
    const x = FakeXHR.last!;
    expect([x.method, x.url, x.withCredentials, x.body]).toEqual(["PUT", "/v1/tenants/t/import-jobs/j1/file", true, file]);
    expect(x.headers).toEqual({ "Content-Type": "application/octet-stream", "X-CSRF-Token": "csrf-1" });
    expect(progress).toEqual([0.5]);
  });

  it("turns a problem into an ApiError with its code, and a 401 into an expired session", async () => {
    vi.stubGlobal("XMLHttpRequest", FakeXHR);
    FakeXHR.respond = () => ({
      status: 413,
      body: JSON.stringify({ type: "urn:glossa:problem:file_too_large", title: "too large", status: 413, code: "file_too_large", detail: "at most 64 MiB" }),
    });
    await expect(apiIntegration.upload("t", importJob, new Blob(["x"]))).rejects.toMatchObject({ status: 413, code: "file_too_large" });
    const expired = vi.fn();
    const off = onSessionExpired(expired);
    FakeXHR.respond = () => ({ status: 401, body: "" });
    await expect(apiIntegration.upload("t", importJob, new Blob(["x"]))).rejects.toBeInstanceOf(ApiError);
    expect(expired).toHaveBeenCalledOnce();
    off();
  });

  it("never sends the session to another origin", async () => {
    vi.stubGlobal("XMLHttpRequest", FakeXHR);
    await expect(apiIntegration.upload("t", { ...importJob, upload_url: "https://evil.example/upload" }, new Blob(["x"]))).rejects.toMatchObject({
      code: "invalid_response",
    });
    expect(FakeXHR.last).toBeUndefined();
  });
});

describe("download", () => {
  it("fetches the file with the session and names it from Content-Disposition", async () => {
    const fetch = vi.fn(
      async (_r: Request) =>
        new Response(new Blob(["PK…"]), {
          status: 200,
          headers: { "Content-Disposition": 'attachment; filename="web.json.zip"', ETag: `"${"a".repeat(64)}"`, "Content-Type": "application/zip" },
        }),
    );
    vi.stubGlobal("fetch", fetch);
    const f = await apiIntegration.download("t", exportJob);
    expect(new URL(fetch.mock.calls[0]![0].url).pathname).toBe("/v1/tenants/t/export-jobs/e1/file");
    expect(f.name).toBe("web.json.zip");
    expect(f.sha256).toBe("a".repeat(64));
    expect(await f.blob.text()).toBe("PK…");
  });

  it("reports an expired file by its code", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => new Response(JSON.stringify({ type: "x", title: "gone", status: 410, code: "file_expired" }), { status: 410, headers: { "Content-Type": "application/problem+json" } })),
    );
    await expect(apiIntegration.download("t", exportJob)).rejects.toMatchObject({ code: "file_expired" });
  });
});
