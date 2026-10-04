.PHONY: help db api web install migrate-up migrate-down test test-api test-web test-e2e lint typecheck build check

help: ## List targets
	@grep -E '^[a-z-]+:.*##' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "  %-14s %s\n", $$1, $$2}'

db: ## Start PostgreSQL in Docker
	docker compose up -d --wait postgres

install: ## Install frontend dependencies
	cd apps/web && npm install

api: ## Run the API (applies migrations in development)
	cd services/api && go run ./cmd/server

web: ## Run the web app in dev mode
	cd apps/web && npm run dev

migrate-up: ## Apply pending migrations
	cd services/api && go run ./cmd/migrate up

migrate-down: ## Roll back ALL migrations (destroys data)
	cd services/api && go run ./cmd/migrate down

test-api: ## Backend tests (set TEST_DATABASE_URL to include PostgreSQL tests)
	cd services/api && go vet ./... && go test -race -count=1 ./...

test-web: ## Frontend unit tests
	cd apps/web && npm test

test-e2e: ## Browser tests; needs the stack running and E2E_ADMIN_EMAIL / E2E_ADMIN_PASSWORD
	cd apps/web && npm run test:e2e

test: test-api test-web ## Unit and API tests

lint: ## Lint and format checks
	cd services/api && test -z "$$(gofmt -l .)" && go vet ./...
	cd apps/web && npm run lint

typecheck: ## Frontend type check
	cd apps/web && npm run typecheck

build: ## Build both applications
	cd services/api && go build -o bin/server ./cmd/server && go build -o bin/migrate ./cmd/migrate
	cd apps/web && npm run build

check: lint typecheck test build ## Everything CI would run
