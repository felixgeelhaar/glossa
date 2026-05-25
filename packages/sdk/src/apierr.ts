// Client-side counterpart to the Go `apierr` package. Defines the
// envelope type that all glossa-aware backends emit on errors, plus a
// resolver that takes an HTTP response body and turns it into the
// localised string to show the user.
//
// Wire shape (matches apierr.Error in github.com/felixgeelhaar/glossa/apierr):
//
//   { "error": {
//       "code":    "validation_email_required",
//       "message": "Email address is required",
//       "key":     "validation.email.required",
//       "params":  { "field": "email" },
//       "status":  400
//   } }
//
// The resolver tries the localised bundle first (via the `messages`
// map a caller passes in), then falls back to the server-supplied
// English `message`. That way clients without a glossa bundle still
// render something useful, and pre-i18n endpoints (those returning a
// plain `{error: "string"}` shape) still produce a readable message.

import { format } from "@felixgeelhaar/glossa-format";

/** Wire shape of one error from a glossa-aware backend. */
export interface ApiErrorPayload {
  code: string;
  message: string;
  key: string;
  params?: Record<string, unknown>;
  status: number;
}

/** The envelope that wraps the error in HTTP responses. */
export interface ApiErrorBody {
  error: ApiErrorPayload;
}

/** Configuration for {@link resolveApiError}. */
export interface ResolveOptions {
  /**
   * The locale's translation bundle — typically the messages map your
   * `<glossa-provider>` already maintains. The resolver looks the
   * error's `key` up here; on miss falls back to `payload.message`.
   */
  messages?: Record<string, string>;
  /**
   * BCP-47 locale tag used by the formatter for plural / number
   * shaping. Defaults to "en" since the server's fallback message is
   * always English.
   */
  locale?: string;
}

/**
 * Resolve a backend error envelope to a user-facing string.
 *
 * Accepts either the full body (`{ error: { ... } }`), the payload
 * directly, or a legacy `{ error: "literal string" }` from
 * pre-apierr endpoints. Returns the locale-resolved string, or a
 * generic "Unknown error" if the payload is unrecognisable — never
 * throws on malformed input, since the caller already has a failing
 * HTTP request to deal with.
 */
export function resolveApiError(
  input: unknown,
  opts: ResolveOptions = {},
): string {
  const payload = extractPayload(input);
  if (!payload) {
    return "Unknown error";
  }
  const { messages = {}, locale = "en" } = opts;
  const template = messages[payload.key] ?? payload.message;
  if (!template) {
    return "Unknown error";
  }
  // format() expects scalar params; coerce anything non-scalar to its
  // string form so the lookup never explodes on unexpected types.
  const params: Record<string, string | number | boolean | null | undefined> =
    {};
  for (const [k, v] of Object.entries(payload.params ?? {})) {
    if (
      v === null ||
      v === undefined ||
      typeof v === "string" ||
      typeof v === "number" ||
      typeof v === "boolean"
    ) {
      params[k] = v;
    } else {
      params[k] = String(v);
    }
  }
  return format(template, locale, params);
}

/**
 * Best-effort extraction: handles the canonical apierr envelope, the
 * bare payload, and the pre-apierr `{ error: "string" }` shape.
 */
function extractPayload(input: unknown): ApiErrorPayload | null {
  if (!input || typeof input !== "object") return null;
  const obj = input as Record<string, unknown>;

  // Legacy `{ error: "literal" }` shape — handle before the canonical
  // envelope check because `obj.error` is a string here, not an object.
  if (typeof obj.error === "string") {
    return {
      code: "unknown_error",
      message: obj.error,
      key: "",
      status: 500,
    };
  }

  // Canonical envelope.
  if (obj.error && typeof obj.error === "object") {
    const e = obj.error as Record<string, unknown>;
    if (typeof e.code === "string" && typeof e.message === "string") {
      return {
        code: e.code,
        message: e.message,
        key: typeof e.key === "string" ? e.key : "",
        params:
          e.params && typeof e.params === "object"
            ? (e.params as Record<string, unknown>)
            : undefined,
        status: typeof e.status === "number" ? e.status : 500,
      };
    }
  }

  // Bare payload (no envelope).
  if (typeof obj.code === "string" && typeof obj.message === "string") {
    return {
      code: obj.code as string,
      message: obj.message as string,
      key: typeof obj.key === "string" ? (obj.key as string) : "",
      params:
        obj.params && typeof obj.params === "object"
          ? (obj.params as Record<string, unknown>)
          : undefined,
      status: typeof obj.status === "number" ? (obj.status as number) : 500,
    };
  }

  return null;
}
