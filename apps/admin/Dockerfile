# syntax=docker/dockerfile:1.6
# Builds the admin SPA from the monorepo root and serves the static
# bundle from nginx. Two stages: pnpm install + vite build, then
# copy `dist/` into an nginx:alpine final image.
#
# Build context is the monorepo root (passed via the workflow) so
# pnpm can resolve workspace deps — apps/admin alone wouldn't see
# packages/ui (and later, packages/{sdk,elements} once they land).

# ── Build stage ──────────────────────────────────────────────────────
FROM node:22-alpine AS build
WORKDIR /repo

# pnpm via corepack to match the workspace's lockfile flavor without
# pinning a global install. Cache the install layer by copying only
# the manifests first.
RUN corepack enable
COPY package.json pnpm-workspace.yaml pnpm-lock.yaml ./
COPY apps/admin/package.json apps/admin/
COPY packages/ui/package.json packages/ui/
RUN pnpm install --frozen-lockfile

# Now copy the source and build. Build the workspace deps the admin
# imports first (currently just @glossa/ui) so its dist/ + type
# declarations are in place when admin's tsc runs.
COPY . .
RUN pnpm --filter @glossa/ui build \
    && pnpm --filter @glossa/admin build

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
