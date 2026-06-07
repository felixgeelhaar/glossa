# syntax=docker/dockerfile:1.6
# Builds the admin SPA and serves the static bundle from nginx.
# Two stages: pnpm install + vite build, then copy `dist/` into an
# nginx:alpine final image.
#
# Admin deps point at the workspace packages (`workspace:^`) so the
# admin always ships with the in-repo @felixgeelhaar/glossa-* code,
# not whatever happens to be on npm. The Dockerfile therefore needs
# the workspace package manifests + sources so pnpm install can
# resolve them locally; the registry is still the source for non-
# workspace deps like Lit.

# ── Build stage ──────────────────────────────────────────────────────
FROM node:22-alpine AS build
WORKDIR /repo

# pnpm via corepack to match the workspace's lockfile flavor without
# pinning a global install. Cache the install layer by copying only
# the manifests first.
RUN corepack enable
COPY package.json pnpm-workspace.yaml pnpm-lock.yaml ./
COPY apps/admin/package.json apps/admin/
COPY packages/elements/package.json packages/elements/
COPY packages/format/package.json packages/format/
COPY packages/sdk/package.json packages/sdk/
COPY packages/ui/package.json packages/ui/
COPY packages/cli/package.json packages/cli/
RUN pnpm install --frozen-lockfile --filter @felixgeelhaar/glossa-admin...

# Sources for workspace deps the admin pulls in; cli isn't a runtime
# dep so it's omitted from this layer.
COPY packages/format packages/format
COPY packages/sdk packages/sdk
COPY packages/ui packages/ui
COPY packages/elements packages/elements
RUN pnpm --filter @felixgeelhaar/glossa-format build \
  && pnpm --filter @felixgeelhaar/glossa-sdk build \
  && pnpm --filter @felixgeelhaar/glossa-ui build \
  && pnpm --filter @felixgeelhaar/glossa-elements build

COPY apps/admin apps/admin
RUN pnpm --filter @felixgeelhaar/glossa-admin build

# ── Runtime stage ────────────────────────────────────────────────────
# nginx-unprivileged runs entirely as a non-root user (uid 101) and
# listens on :8080 (no privileged port, no root master process). The
# custom config serves /usr/share/nginx/html with SPA fallback so
# deep-links survive a refresh (admin uses client-side state).
FROM nginxinc/nginx-unprivileged:alpine
COPY --from=build /repo/apps/admin/dist /usr/share/nginx/html
COPY apps/admin/nginx.conf /etc/nginx/conf.d/default.conf
EXPOSE 8080
# Healthcheck via index.html — nginx returns 200 when the static asset
# is present, which is exactly what readiness should verify.
HEALTHCHECK --interval=10s --timeout=2s --retries=3 \
  CMD wget -qO- http://127.0.0.1:8080/ >/dev/null || exit 1
