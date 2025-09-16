help:
	@awk 'BEGIN {FS = ":.*##"; printf "Usage:\033[36m\033[0m\n"} /^[a-zA-Z_\-/0-9]+:.*?##/ { printf " \033[36m%-40s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

dev-env/up: ## Creates local dev environment
	docker compose -f dev-env/docker-compose.yaml up -d

dev-env/shell: ## Opens shell in local dev environment
	docker compose -f dev-env/docker-compose.yaml exec dev bash

dev-env/down: ## Deletes local dev environment
	docker compose -f dev-env/docker-compose.yaml down -v

fmt: ## Format Go code
	golangci-lint fmt

lint: ## Lint Go code
	golangci-lint run

test/unit: ## Run unit tests with coverage
	go test -coverprofile cover.out ./...
	go tool cover -html=cover.out -o coverage.html

test/race: ## Run unit tests with race detection
	go test -race ./...
