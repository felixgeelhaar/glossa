# `@felixgeelhaar/glossa-sdk`

Framework-agnostic HTTP client + in-memory bundle cache + SSE subscription for [Glossa](https://github.com/felixgeelhaar/glossa). Runs in Node and browsers. ~400 LOC, ~10 KB unpacked, zero non-stdlib deps.

```bash
pnpm add @felixgeelhaar/glossa-sdk
```

## Usage

### Fetch a bundle

```ts
import { createClient } from "@felixgeelhaar/glossa-sdk";

const client = createClient({
  apiUrl: "https://glossa.example.com/api/v1",
  apiKey: "glossa_...",
  project: "brotwerk-site",
});

const bundle = await client.bundle("de");
// → { locale: "de", messages: { "hero.title": "Brotwerk", ... }, etag: "..." }
```

ETag-aware: the second call sends `If-None-Match: <etag>` and returns the cached copy on 304 without re-parsing.

### Subscribe to live updates

```ts
const sub = client.subscribe("de", {
  onUpdate(event) {
    // { type: "translation.updated", key, value, status }
    console.log(event.key, "→", event.value);
  },
  onError(err) {
    console.warn("SSE disconnected:", err);
  },
});

// Later:
sub.close();
```

Each SSE event surgically patches the in-memory cache for that key — no bundle refetch. Reconnects with exponential backoff on transient errors.

### Build-time key sync

```ts
await client.scan({
  keys: [
    { key: "hero.title", description: "Landing hero" },
    { key: "hero.cta_primary" },
  ],
});
```

Used by `@felixgeelhaar/glossa-cli` to seed keys discovered in source files.

## What this package doesn't do

- Render — that's `@felixgeelhaar/glossa-elements`
- Format ICU placeholders — that's `@felixgeelhaar/glossa-format`
- File I/O — that's `@felixgeelhaar/glossa-cli`

## License

MIT
