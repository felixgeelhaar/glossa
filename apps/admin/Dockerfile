# syntax=docker/dockerfile:1.6
# Builds the admin SPA and serves the static bundle from nginx.
# Two stages: pnpm install + vite build, then copy `dist/` into an
# nginx:alpine final image.
#
# Admin deps are pinned to the published @felixgeelhaar/glossa-*
# packages on npm — the Dockerfile no longer needs to copy or build
# the workspace packages. `pnpm install` pulls everything from the
# registry. Local dev still gets workspace linking because the same
# packages live in packages/* with versions that satisfy the
# admin's caret ranges.

# ── Build stage ──────────────────────────────────────────────────────
FROM node:22-alpine AS build
WORKDIR /repo

# pnpm via corepack to match the workspace's lockfile flavor without
# pinning a global install. Cache the install layer by copying only
# the manifests first.
RUN corepack enable
COPY package.json pnpm-workspace.yaml pnpm-lock.yaml ./
COPY apps/admin/package.json apps/admin/
RUN pnpm install --frozen-lockfile --filter @felixgeelhaar/glossa-admin...

COPY apps/admin apps/admin
RUN pnpm --filter @felixgeelhaar/glossa-admin build

# ── Runtime stage ────────────────────────────────────────────────────
# nginx:alpine is small and runs as nonroot by default in alpine
# 1.27+. The default config serves /usr/share/nginx/html on :80; we
# overlay a custom config so SPA deep-links fall back to index.html
# (required because the admin uses client-side state, not a router).
FROM nginx:alpine
COPY --from=build /repo/apps/admin/dist /usr/share/nginx/html
COPY apps/admin/nginx.conf /etc/nginx/conf.d/default.conf
EXPOSE 80
# Healthcheck via the index.html — nginx returns 200 when the
# static asset is present, which is exactly what readiness should
# verify.
HEALTHCHECK --interval=10s --timeout=2s --retries=3 \
  CMD wget -qO- http://127.0.0.1/ >/dev/null || exit 1
