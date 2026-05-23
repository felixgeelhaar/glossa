/// <reference types="vitest" />
import { defineConfig } from "vite";

export default defineConfig({
  test: {
    environment: "happy-dom",
  },
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
