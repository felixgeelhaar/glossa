.PHONY: help api admin packages test lint fmt up down migrate-up migrate-down sqlc-gen

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

packages: ## Build every TS package.
	pnpm -r --filter "./packages/*" build

web-test: ## Run vitest across packages + admin.
	pnpm -r test

# ── Cross-cutting ───────────────────────────────────────────────────
test: api-test web-test ## Backend + frontend tests.

lint: ## Backend (go vet) + frontend (eslint via pnpm).
	cd apps/api && go vet ./...
	pnpm -r lint

fmt: ## gofmt + prettier across the monorepo.
	cd apps/api && gofmt -w .
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
