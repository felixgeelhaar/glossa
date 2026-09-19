.PHONY: help api admin packages test lint fmt up down migrate-up migrate-down sqlc-gen platform-test platform-lint platform-integration system-m2

# Go modules of the rewrite (RFC 0002). apps/api is v0.3 and keeps its
# own targets until it's retired.
PLATFORM_MODULES := messageformat platform runtimes/go runtimes/go/examples/pdf

help: ## Show this help.
	@awk 'BEGIN{FS=":.*## "} /^[a-zA-Z_-]+:.*## / {printf "  %-18s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

# ── Backend ─────────────────────────────────────────────────────────
api: ## Run the Go API locally (requires Postgres at $DATABASE_URL).
	cd apps/api && go run ./cmd/api

api-test: ## Run Go tests.
	cd apps/api && go test ./...

# ── Web (admin + packages) ──────────────────────────────────────────
admin: ## Run the Lit admin UI in dev mode.
	pnpm --filter @glossa/admin dev

packages: ## Build every TS package (topological order).
	pnpm -r --filter "./packages/*" --filter "./messageformat/js" --filter "./runtimes/js/*" build

web-test: ## Run vitest across packages + admin.
	pnpm -r test

# ── Platform (rewrite) ──────────────────────────────────────────────
platform-test: ## Unit tests for every rewrite Go module.
	@for m in $(PLATFORM_MODULES); do (cd $$m && go test ./...) || exit 1; done

platform-lint: ## go vet for every rewrite Go module.
	@for m in $(PLATFORM_MODULES); do (cd $$m && go vet ./...) || exit 1; done

platform-integration: ## Docker-backed integration tests (Postgres, object storage).
	@for m in $(PLATFORM_MODULES); do (cd $$m && go test -tags=integration -timeout=300s ./...) || exit 1; done

system-m2: ## M2 exit test (Docker): fill es/fr/ja through glossa-server; writes platform/internal/systemtest/m2/REPORT.md.
	cd platform && go test -tags=system -timeout=600s -count=1 -v ./internal/systemtest/...

# ── Cross-cutting ───────────────────────────────────────────────────
test: api-test platform-test web-test ## Backend + frontend tests.

lint: platform-lint ## Backend (go vet) + frontend (tsc via pnpm).
	cd apps/api && go vet ./...
	pnpm -r lint

fmt: ## gofmt + prettier across the monorepo.
	cd apps/api && gofmt -w .
	@for m in $(PLATFORM_MODULES); do (cd $$m && gofmt -w .); done
	pnpm -r format

# ── Docker compose ──────────────────────────────────────────────────
up: ## Bring up the dev Postgres + API + admin via compose.
	docker compose up -d

down:
	docker compose down

# ── DB / codegen (placeholders until apps/api lands) ────────────────
migrate-up: ## Apply DB migrations.
	cd apps/api && go run ./cmd/migrate up

migrate-down: ## Roll back one DB migration.
	cd apps/api && go run ./cmd/migrate down

sqlc-gen: ## Regenerate sqlc bindings.
	cd apps/api && go tool sqlc generate
