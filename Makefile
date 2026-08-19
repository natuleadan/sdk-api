.PHONY: all build test test-unit test-integration test-coverage lint clean third-party security-deps security-sast security-sbom security-audit bench bench-compare pgo-profile

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
	@echo "Waiting for Zitadel readiness (host healthz)..."
	@for i in $$(seq 1 30); do \
		if curl -sf http://localhost:18082/debug/healthz >/dev/null 2>&1; then \
			echo "Zitadel ready (attempt $$i)"; break; \
		fi; \
		if [ $$i -eq 30 ]; then echo "Zitadel not ready after 30 attempts"; fi; \
		sleep 2; \
	done
	@echo "Running integration tests..."
	DATABASE_URL="postgres://dev:devpass@localhost:15432/postgres?sslmode=disable" \
	NATS_URL="nats://localhost:14222" \
	KAFKA_URL="localhost:9092" \
	MYSQL_URL="test:pass@tcp(localhost:13306)/test?parseTime=true" \
	go test -race -v -count=1 -tags=integration ./...
	@echo "Stopping test services..."
	docker compose -f docker-compose.test.yml down

test: test-unit test-integration

clean:
	go clean -cache -testcache

third-party:
	@bash .github/scripts/generate-third-party.sh

bench:
	@echo "Running benchmarks..."
	@go test -bench=. -benchmem -count=5 -benchtime=100ms ./runtime/benchmarks/...

bench-compare:
	@echo "Running benchmarks with comparison to main..."
	@tmp_main=$$(mktemp); tmp_pr=$$(mktemp); \
	go test -bench=. -benchmem -count=10 -benchtime=100ms ./runtime/benchmarks/... | tee $$tmp_pr; \
	git stash && git checkout main 2>/dev/null || true; \
	go test -bench=. -benchmem -count=10 -benchtime=100ms ./runtime/benchmarks/... | tee $$tmp_main; \
	git checkout - 2>/dev/null && git stash pop 2>/dev/null || true; \
	if command -v benchstat &>/dev/null; then \
		echo "=== benchstat comparison ==="; \
		benchstat $$tmp_main $$tmp_pr; \
	else \
		echo "benchstat not installed; install with: go install golang.org/x/perf/cmd/benchstat@latest"; \
	fi; \
	rm -f $$tmp_main $$tmp_pr

pgo-profile:
	@echo "To collect a PGO profile from a running service:"
	@echo "  curl -o default.pgo 'http://localhost:6060/debug/pprof/profile?seconds=30'"
	@echo "  cp default.pgo cmd/sdk-api/default.pgo"
	@echo ""
	@echo "Or use a production endpoint:"
	@echo "  curl -o default.pgo '$(PROD_URL)/debug/pprof/profile?seconds=30'"
	@echo "  cp default.pgo cmd/sdk-api/default.pgo"
	@echo ""
	@echo "The Go compiler detects default.pgo automatically on next build."
	@echo "Verify with: go build -o /dev/null ./cmd/sdk-api 2>&1 | grep -q 'building with PGO' && echo 'PGO enabled'"

pgo-verify:
	@go build -o /dev/null ./cmd/sdk-api 2>&1 | grep -q "building with PGO" && \
		echo "✅ PGO enabled" || echo "⚠️  PGO not applied (no default.pgo found)"

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

# ---- Secrets Management (SOPS) ----

# Decrypt a SOPS-encrypted config file
# Usage: make decrypt-config FILE=service.enc.yaml
decrypt-config:
	@if ! command -v sops >/dev/null 2>&1; then \
		echo "Error: sops not found. Install: brew install sops"; \
		exit 1; \
	fi
	sops --decrypt --input-type yaml --output-type yaml $(FILE)

# Encrypt a plain config file with SOPS
# Usage: make encrypt-config FILE=service.yaml
encrypt-config:
	@if ! command -v sops >/dev/null 2>&1; then \
		echo "Error: sops not found. Install: brew install sops"; \
		exit 1; \
	fi
	sops --encrypt --input-type yaml --output-type yaml $(FILE) > $(FILE:.yaml=.enc.yaml)

golangci:
	@echo "Running golangci-lint..."
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	golangci-lint run ./...

.DEFAULT_GOAL := all
