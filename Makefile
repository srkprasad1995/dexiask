# Dexiask — top-level orchestration. Run `make help` for the list.
.DEFAULT_GOAL := help
COMPOSE := docker compose

.PHONY: help up down build logs ps restart test test-backend test-memory test-engine test-indexer test-web lint fmt clean

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}'

up: ## Build (if needed) and start the whole stack
	$(COMPOSE) up --build -d
	@echo "web → http://localhost:$${DEXIASK_WEB_PORT:-25051}"

down: ## Stop the stack
	$(COMPOSE) down

build: ## Build all images
	$(COMPOSE) build

logs: ## Tail logs for all services
	$(COMPOSE) logs -f

ps: ## Show service status
	$(COMPOSE) ps

restart: down up ## Restart the stack

# ---- Tests (run each suite in its own toolchain) --------------------------
test: test-backend test-memory test-engine test-indexer test-web ## Run every test suite

test-backend: ## Go unit tests
	cd backend && go test ./...

test-memory: ## Memory service (Go) unit tests
	cd memory && go test ./...

test-engine: ## Python engine tests
	cd engine && python -m pytest -q

test-indexer: ## Python indexer tests
	cd indexer && python -m pytest -q

test-web: ## Web (vitest) tests
	cd web && pnpm test

lint: ## Lint everything
	cd backend && go vet ./...
	cd memory && go vet ./...
	cd engine && ruff check .
	cd indexer && ruff check .
	cd web && pnpm lint

fmt: ## Format Go + Python
	cd backend && gofmt -w .
	cd memory && gofmt -w .
	cd engine && ruff format .
	cd indexer && ruff format .

clean: ## Stop stack and remove volumes (DESTROYS the DB + index)
	$(COMPOSE) down -v
