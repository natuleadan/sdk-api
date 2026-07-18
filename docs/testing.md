# Testing

## Running tests

```bash
# Unit tests (fast, no Docker)
make test-unit

# Integration tests (requires Docker: PostgreSQL + NATS + Kafka)
make test-integration

# All tests
make test

# Coverage report
make test-coverage

# Fuzzing (30s per target)
go test -fuzz=FuzzConfigParse -fuzztime=30s ./runtime/
```

## Build tags

| Tag | When to use | How to run |
|-----|-------------|------------|
| `//go:build integration` | Tests requiring Docker (PG, NATS, Kafka, Zitadel, OpenFGA) | `go test -tags=integration` |
| (none) | Pure unit tests | `go test -short` |

Files tagged with `//go:build integration`:
- `runtime/nats_integration_test.go`
- `events/events_test.go`, `events/producer_test.go`, `events/kafka_broker_test.go`
- `db/db_test.go`, `db/mysql_test.go`
- `server/auth/zitadel/client_integration_test.go`
- `server/auth/openfga/client_integration_test.go`

Tests that require infrastructure but have runtime skip conditions (no build tag):
- `runtime/exit_test.go` — skips if `NATS_URL` not set
- `runtime/db_test.go:TestCheckPoolHealth_Postgres` — skips in short mode

## Assertions

Use testify assertions instead of raw `t.Errorf` / `t.Fatal`:

```go
// ✅ Correct
assert.Equal(t, 200, resp.StatusCode)
require.NoError(t, err)
require.Equal(t, "expected", got)
```

```go
// ❌ Avoid
if resp.StatusCode != 200 { t.Errorf("expected 200, got %d", resp.StatusCode) }
if err != nil { t.Fatal(err) }
```

## Goroutine leak detection

`runtime/main_test.go` and `events/main_test.go` use `goleak.VerifyTestMain` to detect goroutine leaks after all tests in the package complete.

Known background goroutines (go-zero infra, fasthttp, HTTP transport, rate limiter GC) are ignored via `goleak.IgnoreAnyFunction`. Any new goroutine not in the ignore list will cause a test failure.

To add a new ignore rule for an expected background goroutine:
```go
goleak.IgnoreAnyFunction("github.com/natuleadan/sdk-api/infra/somepackage.SomeFunc")
```

## Parallel tests

Every test function should call `t.Parallel()` at the start. This is safe because:

- `logx.Disable()` uses atomic operations (thread-safe)
- Each test creates its own `fiber.New()` app (isolated state)
- `app.Test()` uses in-memory pipes, no port binding

```go
func TestSomething(t *testing.T) {
    t.Parallel()
    // ...
}
```

## Fuzzing

Fuzz tests discover edge cases automatically in parsers and validators:

| Fuzz function | Target | Location |
|---------------|--------|----------|
| `FuzzConfigParse` | YAML config parsing | `runtime/fuzz_test.go` |
| `FuzzSanitizeKey` | Key sanitization (path traversal) | `runtime/fuzz_test.go` |
| `FuzzParseMaxSize` | File size limit parsing | `runtime/fuzz_test.go` |

Run fuzzing:
```bash
go test -fuzz=FuzzConfigParse -fuzztime=30s ./runtime/
go test -fuzz=FuzzSanitizeKey -fuzztime=30s ./runtime/
go test -fuzz=FuzzParseMaxSize -fuzztime=30s ./runtime/
```

Fuzz tests should verify:
- No panics on arbitrary input
- No path traversal in output
- No negative values from parsers

## Coverage

Threshold is set in `Makefile`:
```makefile
COVERAGE_THRESHOLD = 50
```

Run coverage check:
```bash
make test-coverage
```

Current coverage: ~65%. The threshold should be increased progressively as new tests are added.

## Testify suites

For tests that share setup (same `fiber.App`, same DB pool), use `suite.Suite`:

```go
type MiddlewareSuite struct {
    suite.Suite
    app *fiber.App
}

func (s *MiddlewareSuite) SetupTest() {
    logx.Disable()
    s.app = fiber.New()
}

func (s *MiddlewareSuite) TestSomething() {
    s.app.Get("/test", handler)
    resp, _ := s.app.Test(req)
    s.Equal(200, resp.StatusCode)
}

func TestMiddlewareSuite(t *testing.T) {
    suite.Run(t, new(MiddlewareSuite))
}
```

See `server/middleware/suite_test.go` for a working example.

## Integration test infrastructure

The `docker-compose.test.yml` file defines:

| Service | Default port | Env var |
|---------|-------------|---------|
| PostgreSQL | 15432 | `DATABASE_URL` |
| NATS | 14222 | `NATS_URL` |
| Kafka | 9092 | `KAFKA_URL` |

Run integration tests:
```bash
make test-integration
```
