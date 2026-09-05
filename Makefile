# Cuckoo — developer commands.
#
# Everyday loop:   make up   (once)  →  make dev  →  make check  (before commit)

SHELL := /bin/bash
.DEFAULT_GOAL := help

SERVER_DIR := server
BIN_DIR    := $(CURDIR)/bin
export PATH := $(BIN_DIR):$(PATH)

# Pinned tool versions. Installed into ./bin by `make tools` — never global.
SQLC_VERSION          := v1.31.1
AIR_VERSION           := v1.63.0
GOLANGCI_LINT_VERSION := v2.13.2

# Enforced by scripts/check-file-size.sh and `make check`.
export CUCKOO_FILE_LINE_LIMIT := 500

DEV_DATABASE_URL  := postgres://cuckoo:cuckoo@localhost:5433/cuckoo?sslmode=disable
TEST_DATABASE_URL := postgres://cuckoo:cuckoo@localhost:5433/cuckoo_test?sslmode=disable

##@ Help

.PHONY: help
help: ## Show this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nCuckoo\n\nUsage:\n  make \033[36m<target>\033[0m\n"} \
		/^[a-zA-Z0-9_-]+:.*?##/ { printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2 } \
		/^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) }' $(MAKEFILE_LIST)
	@echo

##@ Environment

.PHONY: up
up: ## Start Postgres and Redis in Docker
	docker compose up -d
	@echo "waiting for postgres..."
	@until docker compose exec -T postgres pg_isready -U cuckoo -q; do sleep 0.5; done
	@echo "ready. dev db: $(DEV_DATABASE_URL)"

.PHONY: down
down: ## Stop containers (keeps data)
	docker compose down

.PHONY: reset
reset: ## Stop containers and DELETE all local data
	docker compose down -v

.PHONY: logs
logs: ## Tail container logs
	docker compose logs -f

.PHONY: psql
psql: ## Open a psql shell on the dev database
	docker compose exec postgres psql -U cuckoo -d cuckoo

.PHONY: tools
tools: ## Install pinned dev tools into ./bin
	@mkdir -p $(BIN_DIR)
	GOBIN=$(BIN_DIR) go install github.com/sqlc-dev/sqlc/cmd/sqlc@$(SQLC_VERSION)
	GOBIN=$(BIN_DIR) go install github.com/air-verse/air@$(AIR_VERSION)
	GOBIN=$(BIN_DIR) go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
	@echo "installed into $(BIN_DIR)"

##@ Develop

.PHONY: dev
dev: ## Run the server with hot reload (needs `make up` and `make tools`)
	cd $(SERVER_DIR) && air

.PHONY: run
run: ## Run the server once, no hot reload
	cd $(SERVER_DIR) && go run ./cmd/cuckoo

.PHONY: build
build: ## Compile the server binary into ./bin
	@mkdir -p $(BIN_DIR)
	cd $(SERVER_DIR) && go build -o $(BIN_DIR)/cuckoo ./cmd/cuckoo
	@echo "built $(BIN_DIR)/cuckoo"

.PHONY: gen
gen: ## Regenerate typed DB code from SQL (sqlc)
	cd $(SERVER_DIR) && sqlc generate
	@echo "regenerated server/internal/store/gen"

.PHONY: fmt
fmt: ## Format Go code
	cd $(SERVER_DIR) && go fmt ./...

##@ Mobile

.PHONY: mobile-install
mobile-install: ## Install the app's dependencies
	cd apps/mobile && npm install --no-audit --no-fund

.PHONY: mobile
mobile: ## Start the app (set CUCKOO_HUB_URL to the hub's address on your wifi)
	cd apps/mobile && npx expo start

.PHONY: mobile-check
mobile-check: ## Typecheck, lint, format-check and test the app
	cd apps/mobile && npm run check

.PHONY: play
play: ## Start everything for a phone test (make play EMAIL=you@example.com)
	@scripts/play.sh $(EMAIL)

.PHONY: say
say: ## Send a message as yourself to your Echo agent (make say TEXT="hello")
	@scripts/play.sh say "$(TEXT)"

.PHONY: stop
stop: ## Stop anything make play left running
	@scripts/play.sh stop

##@ Verify

.PHONY: test
test: size ## Run all tests (includes the file-size guard)
	cd $(SERVER_DIR) && CUCKOO_TEST_DATABASE_URL="$(TEST_DATABASE_URL)" go test ./... -race -count=1

.PHONY: test-short
test-short: ## Run only tests that need no database
	cd $(SERVER_DIR) && go test ./... -short -count=1

.PHONY: cover
cover: ## Run tests and open an HTML coverage report
	cd $(SERVER_DIR) && CUCKOO_TEST_DATABASE_URL="$(TEST_DATABASE_URL)" \
		go test ./... -coverprofile=coverage.out -count=1
	cd $(SERVER_DIR) && go tool cover -html=coverage.out

.PHONY: lint
lint: ## Run the Go linter
	cd $(SERVER_DIR) && golangci-lint run

.PHONY: size
size: ## Fail if any source file exceeds the line limit
	@scripts/check-file-size.sh

.PHONY: size-top
size-top: ## Show the longest source files
	@scripts/check-file-size.sh --top 15

.PHONY: secrets
secrets: ## Fail if a credential is about to be committed
	@scripts/check-secrets.sh --all

.PHONY: check
check: size secrets lint test ## Full verification — run this before every commit

##@ Local CI

.PHONY: ci
ci: ## Everything a CI server would run, including a migrate-from-empty check
	@scripts/ci.sh

.PHONY: watch
watch: ## Rerun tests on every file change (WATCH_FULL=1 to include the database)
	@scripts/watch.sh $(ARGS)

.PHONY: hooks
hooks: ## Install the git hooks (pre-commit and pre-push)
	@git config core.hooksPath scripts/hooks
	@echo "hooks installed — pre-commit (~1s) and pre-push (~5s)"
	@echo "bypass a single time with --no-verify; run 'make unhook' to remove"

.PHONY: unhook
unhook: ## Remove the git hooks
	@git config --unset core.hooksPath || true
	@echo "hooks removed"

.PHONY: clean
clean: ## Remove build output
	rm -rf $(BIN_DIR) $(SERVER_DIR)/tmp $(SERVER_DIR)/coverage.out
