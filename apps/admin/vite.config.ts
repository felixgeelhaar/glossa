import { defineConfig } from "vite";

// Tests are configured in vitest.config.ts, which vitest prefers over this
// file. The `test.environment: "happy-dom"` that used to sit here was dead —
// contradicted by the jsdom environment next door — and its only effect was
// pulling happy-dom 15 into the lockfile as an optional peer, where it sat
// with a critical advisory against it.
export default defineConfig({
  server: {
    port: 5173,
    // Proxy /api and /api/v1/.../sse to the Go API in dev so the
    // SPA can run on a different port without CORS gymnastics.
    proxy: {
      "/api": {
        target: "http://localhost:8080",
        changeOrigin: true,
        ws: false,
      },
    },
  },
  build: {
    target: "es2022",
    sourcemap: true,
  },
});
