.PHONY: all build test test-unit test-integration lint clean third-party

all: build lint test-unit

build: third-party
	go build ./...

lint:
	go vet ./...

test-unit:
	go test -race -count=1 ./cmd/sdk-api/... ./db/... ./server/... ./infra/... ./internal/...

test-integration:
	@echo "Starting test services (PostgreSQL + NATS)..."
	docker compose -f docker-compose.test.yml up -d --wait 2>/dev/null || true
	@echo "Waiting for services..."
	@sleep 5
	@echo "Running integration tests..."
	DATABASE_URL="postgres://dev:devpass@localhost:15432/postgres?sslmode=disable" \
	NATS_URL="nats://localhost:14222" \
	go test -race -v -count=1 ./runtime/... ./events/... ./db/...
	@echo "Stopping test services..."
	docker compose -f docker-compose.test.yml down

test: test-unit test-integration

clean:
	go clean -cache -testcache

third-party:
	@echo "Generating ThirdPartyNotices.txt..."
	@go run github.com/google/go-licenses@latest csv ./... 2>/dev/null | grep -v "natuleadan" | \
	  awk -F, '{printf "- %s (%s)\n  %s\n\n", $$1, $$3, $$2}' > ThirdPartyNotices.txt
	@echo "Done"

.DEFAULT_GOAL := all
