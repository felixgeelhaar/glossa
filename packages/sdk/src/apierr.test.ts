import { describe, it, expect } from "vitest";
import { resolveApiError } from "./apierr.js";

describe("resolveApiError", () => {
  it("interpolates params from the localised bundle when the key is known", () => {
    const body = {
      error: {
        code: "validation_email_required",
        message: "Email address is required",
        key: "validation.email.required",
        params: { field: "email" },
        status: 400,
      },
    };
    const got = resolveApiError(body, {
      locale: "de",
      messages: {
        "validation.email.required": "{field} ist erforderlich",
      },
    });
    expect(got).toBe("email ist erforderlich");
  });

  it("falls back to the server-supplied English message on bundle miss", () => {
    const body = {
      error: {
        code: "unknown",
        message: "Something broke",
        key: "errors.unknown",
        status: 500,
      },
    };
    expect(resolveApiError(body)).toBe("Something broke");
  });

  it("falls back to the server message when no key was set", () => {
    const body = {
      error: {
        code: "x",
        message: "x failed",
        key: "",
        status: 500,
      },
    };
    expect(resolveApiError(body, { messages: { "errors.x": "irrelevant" } }))
      .toBe("x failed");
  });

  it("accepts a bare payload without the envelope", () => {
    const payload = {
      code: "rate_limited",
      message: "Slow down",
      key: "errors.rate_limited",
      status: 429,
    };
    expect(resolveApiError(payload)).toBe("Slow down");
  });

  it("handles the legacy { error: 'literal' } shape", () => {
    expect(resolveApiError({ error: "boom" })).toBe("boom");
  });

  it("returns 'Unknown error' for unrecognisable input rather than throwing", () => {
    expect(resolveApiError(null)).toBe("Unknown error");
    expect(resolveApiError({})).toBe("Unknown error");
    expect(resolveApiError({ wrong: "shape" })).toBe("Unknown error");
  });
});
