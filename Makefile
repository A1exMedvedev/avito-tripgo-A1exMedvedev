ENV_FILE ?= .env
-include $(ENV_FILE)
export

MIGRATIONS_DIR := migrations
GOOSE = go tool goose -dir $(MIGRATIONS_DIR) postgres "$(DATABASE_URL)"

GOLANGCI_LINT = go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0
COVER_PACKAGES = ./internal/domain/...,./internal/usecase/...

.PHONY: generate migrate migrate-down migrate-reset migrate-status run build test test-integration lint fmt docker-build docker-run check-db-url

generate:
	go tool oapi-codegen -config api/oapi-codegen.yaml contracts/openapi/trip-service.openapi.yaml
	go generate ./...

migrate: check-db-url
	$(GOOSE) up

migrate-down: check-db-url
	$(GOOSE) down

migrate-reset: check-db-url
	$(GOOSE) reset

migrate-status: check-db-url
	$(GOOSE) status

run:
	go run ./cmd/trip-service

build:
	go build -o bin/trip-service ./cmd/trip-service

test:
	go test -race -count=1 -coverpkg=$(COVER_PACKAGES) -coverprofile=coverage.out ./...
	@echo "coverage of internal/domain and internal/usecase:"
	@go tool cover -func=coverage.out | tail -n 1

test-integration:
	go test -race -count=1 -tags=integration ./internal/repository/...

lint:
	$(GOLANGCI_LINT) run ./...

fmt:
	$(GOLANGCI_LINT) fmt ./...

IMAGE ?= trip-service:local

docker-build:
	docker build -f deploy/Dockerfile -t $(IMAGE) .

docker-run:
	docker run --rm --network host --env-file $(ENV_FILE) $(IMAGE)

check-db-url:
	@test -n "$(DATABASE_URL)" || (echo "DATABASE_URL is not set: run tripgoctl environment start or create $(ENV_FILE)" && exit 1)
