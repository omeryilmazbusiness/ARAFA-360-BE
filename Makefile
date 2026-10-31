SHELL := /bin/bash
BIN := bin/arafa-360
IMAGE ?= arafa-360:local

# Load .env (if present) into every recipe: `make run` needs no manual exports.
ifneq (,$(wildcard .env))
include .env
export
endif

.DEFAULT_GOAL := help
.PHONY: help setup up down run dev build test test-race cover vet fmt tidy seed psql redis-cli docker-build docker-run clean

help: ## Show this help
	@awk 'BEGIN{FS=":.*##"} /^[a-zA-Z_-]+:.*##/ {printf "  \033[36m%-14s\033[0m %s\n",$$1,$$2}' $(MAKEFILE_LIST)

setup: ## First-time setup: .env + Postgres/Redis containers
	@test -f .env || cp .env.example .env
	@$(MAKE) up

up: ## Start Postgres + Redis (docker compose)
	docker compose up -d --wait

down: ## Stop Postgres + Redis
	docker compose down

run: ## Run the API (auto-migrates)
	go run .

dev: ## Hot reload with air (go install github.com/air-verse/air@latest)
	@command -v air >/dev/null || { echo "install air: go install github.com/air-verse/air@latest"; exit 1; }
	air --build.cmd "go build -o tmp/main ." --build.bin "./tmp/main"

build: ## Build a local optimised binary into ./bin
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o $(BIN) .

test: ## Run unit tests
	go test ./...

test-race: ## Run unit tests with the race detector
	go test -race -count=1 ./...

cover: ## Coverage report
	go test -race -coverprofile=coverage.out ./... && go tool cover -func=coverage.out | tail -1

vet: ## go vet
	go vet ./...

fmt: ## gofmt
	gofmt -s -w $$(git ls-files '*.go' 2>/dev/null || find . -name '*.go' -not -path './_legacy_kernel/*')

tidy: ## go mod tidy
	go mod tidy

seed: ## Insert the demo tenant + zone
	psql "$(DATABASE_URL)" -v ON_ERROR_STOP=1 -f scripts/seed.sql

psql: ## Open psql on DATABASE_URL
	psql "$(DATABASE_URL)"

redis-cli: ## Open redis-cli on REDIS_URL
	redis-cli -u "$(REDIS_URL)"

docker-build: ## Build the production image (same Dockerfile as Railway)
	docker build -t $(IMAGE) .

docker-run: docker-build ## Run the image against the compose services
	docker run --rm -p 8080:8080 --env-file .env \
	  -e DATABASE_URL=postgres://arafa:arafa@host.docker.internal:5435/arafa360?sslmode=disable \
	  -e REDIS_URL=redis://host.docker.internal:6381/0 $(IMAGE)

clean: ## Remove build artefacts
	rm -rf bin tmp coverage.out
