---
"@felixgeelhaar/glossa-sdk": minor
---

Add `resolveApiError(payload, opts?)` plus the matching `ApiErrorBody`, `ApiErrorPayload`, and `ResolveOptions` types. Takes the JSON envelope emitted by glossa-aware Go backends via the new `github.com/felixgeelhaar/glossa/apierr` Go module (`{ error: { code, message, key, params?, status } }`), looks the `key` up in a provided messages map, and renders the result via the existing glossa-format interpolator. Falls back gracefully to the server-supplied English `message` on bundle miss, to the legacy `{ error: "literal" }` shape, and to `"Unknown error"` on malformed input — never throws so it's safe to call from a failing-fetch path.
