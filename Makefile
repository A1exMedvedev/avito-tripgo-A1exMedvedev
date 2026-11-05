ENV_FILE ?= .env
-include $(ENV_FILE)
export

MIGRATIONS_DIR := migrations
GOOSE = go tool goose -dir $(MIGRATIONS_DIR) postgres "$(DATABASE_URL)"

.PHONY: generate migrate migrate-down migrate-reset migrate-status run build test docker-build docker-run check-db-url

generate:
	go tool oapi-codegen -config api/oapi-codegen.yaml contracts/openapi/trip-service.openapi.yaml

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
	go test -race ./...

IMAGE ?= trip-service:local

docker-build:
	docker build -f deploy/Dockerfile -t $(IMAGE) .

docker-run:
	docker run --rm --network host --env-file $(ENV_FILE) $(IMAGE)

check-db-url:
	@test -n "$(DATABASE_URL)" || (echo "DATABASE_URL is not set: run tripgoctl environment start or create $(ENV_FILE)" && exit 1)
