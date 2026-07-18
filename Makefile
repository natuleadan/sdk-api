.PHONY: all build test test-unit test-integration test-coverage lint clean third-party security-deps security-sast security-sbom security-audit

COVERAGE_THRESHOLD = 50

all: build lint test-unit

build: third-party
	go build ./...

lint:
	golangci-lint run ./... --timeout=5m
	go vet ./...

test-unit:
	go test -race -count=1 -short ./...

test-coverage:
	go test -short -coverprofile=coverage.out -covermode=atomic ./...
	@coverage=$$(go tool cover -func=coverage.out | grep total | awk '{print $$3}'); \
	echo "Coverage: $$coverage"; \
	if [ "$${coverage%.*}" -lt $(COVERAGE_THRESHOLD) ]; then \
		echo "FAIL: coverage $(COVERAGE_THRESHOLD)% threshold not met ($$coverage)"; \
		exit 1; \
	fi

test-integration:
	@echo "Starting test services..."
	docker compose -f docker-compose.test.yml up -d --wait 2>/dev/null || true
	@echo "Waiting for services..."
	@sleep 5
	@echo "Running integration tests..."
	DATABASE_URL="postgres://dev:devpass@localhost:15432/postgres?sslmode=disable" \
	NATS_URL="nats://localhost:14222" \
	KAFKA_URL="localhost:9092" \
	MYSQL_URL="root:pass@tcp(localhost:3306)/test" \
	go test -race -v -count=1 -tags=integration ./...
	@echo "Stopping test services..."
	docker compose -f docker-compose.test.yml down

test: test-unit test-integration

clean:
	go clean -cache -testcache

third-party:
	@bash .github/scripts/generate-third-party.sh

# --- Security Scanning ---

security-deps:
	@echo "Scanning dependencies for vulnerabilities..."
	go install golang.org/x/vuln/cmd/govulncheck@latest
	govulncheck ./...

security-sast:
	@echo "Running static analysis security testing (SAST)..."
	go install github.com/securego/gosec/v2/cmd/gosec@latest
	gosec -quiet ./...

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
