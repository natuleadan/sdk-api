.PHONY: all build test test-unit test-integration lint clean third-party security-deps security-sast security-sbom security-audit

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

# --- Security Scanning ---

security-deps:
	@echo "Scanning dependencies for vulnerabilities..."
	go install golang.org/x/vuln/cmd/govulncheck@latest
	govulncheck ./...

security-sast:
	@echo "Running static analysis security testing (SAST)..."
	go install github.com/securego/gosec/v2/cmd/gosec@latest
	gosec -quiet -exclude=G304,G307 -exclude-dir=testdata ./...

security-sbom:
	@echo "Generating SBOM (Software Bill of Materials)..."
	go install github.com/anchore/syft/cmd/syft@latest
	syft . -o spdx-json > sbom.spdx.json 2>/dev/null || echo "Syft not available, install via: brew install syft"

security-audit: security-deps security-sast security-sbom
	@echo "Security audit complete"

golangci:
	@echo "Running golangci-lint..."
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	golangci-lint run ./...

.DEFAULT_GOAL := all
