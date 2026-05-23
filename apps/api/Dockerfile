# syntax=docker/dockerfile:1.6
# ── Build stage ──────────────────────────────────────────────────────
FROM golang:1.26-alpine AS build
WORKDIR /src
RUN apk add --no-cache git ca-certificates
COPY go.mod ./
COPY go.su[m] ./
RUN go mod download || true
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/glossa-api ./cmd/api

# golang-migrate, postgres-only build. We bundle it into the runtime
# image so the `migrate` initContainer in the k3s api Deployment can
# `/migrate up` against $DATABASE_URL without a separate image. Same
# trick as Brotwerk — keeps the migration story identical across
# projects and avoids dragging in S3/GCS drivers we never use.
ARG MIGRATE_VERSION=v4.18.1
RUN go install -tags 'postgres' \
    github.com/golang-migrate/migrate/v4/cmd/migrate@${MIGRATE_VERSION} \
    && cp "$(go env GOPATH)/bin/migrate" /out/migrate-bin \
    && chmod +x /out/migrate-bin

# ── Runtime stage ────────────────────────────────────────────────────
# Distroless static so the image is small, has no shell to be popped,
# and runs nonroot by default. The api binary is statically linked
# (CGO_ENABLED=0) so we don't need glibc.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/glossa-api /glossa-api
COPY --from=build /out/migrate-bin /migrate
COPY --from=build /src/db/migrations /migrations
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/glossa-api"]
