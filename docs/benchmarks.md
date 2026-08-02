# Benchmarks

How to measure and maximize RPS with the sdk-api framework.

## Rules

1. **All benchmarks run fully inside Docker.** Running the Go binary on the host while data services run in Docker adds 2-4x latency due to Docker Desktop port mapping. The `wrk` tool runs inside the same container as the service — never on the macOS host.
2. **Use wrk, not Go testing.B** for high-concurrency benchmarks. Go goroutines have scheduling overhead at 1000+ concurrency.
3. **Each folder uses `docker compose run`** via `run.sh` which accepts:
   - `--rps`: run functional tests then RPS benchmarks (wrk inside container, 3s warmup + 5s measure)
   - `TestName`: run a specific test (e.g., `./run.sh TestHealthz_OK`)
   - `--test:Name`: same as `TestName` (e.g., `--test:TestHealthz_OK`)
 4. **PostgreSQL max_connections must match pool size.** Use `command: ["postgres", "-c", "max_connections=500"]` with PgDog managing the pool (default_pool_size 200, min_pool_size 50, workers 8).
5. **Results are maintained in each example's README.** Re-run the benchmark to verify. Update the README if the result changes.
6. **Functional tests run by default** (no flags needed). The container entrypoint (`run.sh`) runs the test binary for that variant.
7. **Each example seeds hot keys** before the RPS benchmark (via curl POST). 200 for URL shortener, 50 for pg-nats, etc. This ensures caches are warm and every request hits the fast path.
8. **Endpoints are measured sequentially** — 2–8 depending on the variant. Each endpoint gets 30s warmup + 30s measurement.
9. **wrk runs INSIDE the container, not on macOS host.** Running wrk from macOS against a Docker container adds virtualisation overhead and produces invalid RPS numbers. Use `--rps` (not `--local --rps`) for official benchmarks.
10. **CRUD update benchmarks must use `PATCH` (the method the CRUD entry registers), never `PUT`.** A `PUT` request to a PATCH-only route returns `405 Method Not Allowed` without touching the database, inflating RPS by 10-20x and producing invalid numbers (all pre-2026-08-01 `Update` rows above ~20K are 405 artifacts).
11. **Update benchmarks must spread writes across more rows than concurrent clients.** With `wrk -t10 -c1000`, a `math.random(1, 200)` range against 200 hot rows causes row-lock contention that throttles real updates to ~9K RPS. Use `math.random(1, 2000)` (ids 1-2000 exist after the create-phase seed) so the number reflects update throughput, not lock collisions.

## Maximizing RPS

### 1. Prefork

Enable `prefork: true` for multi-core throughput when the bottleneck is CPU-bound (middleware chain, JSON serialization, cache hits).
When the bottleneck is the database, prefork does not improve throughput — all processes compete for the same DB connections.

### 2. Middleware

The standard middleware stack (logger, shedding, breaker, maxconns, maxbytes, gunzip, prometheus, cors) has minimal overhead on simple endpoints. For maximum throughput:

```yaml
server:
  middleware:
    - path: "/api/v1/*"
      apply: []
```

This disables the 8 standard middlewares (logger, shedding, breaker, maxconns, maxbytes, gunzip, prometheus, cors) per-route. Four middlewares are always active (recover, header sanitize, health endpoint, metrics endpoint) with negligible overhead.

### 3. Connection Pool

- **PgDog** prevents connection storms from 1000-concurrent-wrk × 10 prefork processes.
- PgDog pool size: `200`. PostgreSQL `max_connections`: `500`.
- Without a pooler, set reasonable `max_conns` on the application pool (e.g. mariadb app pool `100`).

### 3b. KV Store Tuning (Dragonfly)

- Run Dragonfly with `--cluster_mode=emulated --proactor_threads=2 --maxclients=20000`. The 07-11 default (all threads) throttled create to ~29K; with proactor 2 it reaches ~34K and lifts L2-cache endpoints (expand) by ~44%.
- `kv.pool_size` is YAML-driven (`kv:` entries). `0` (default) uses the go-redis default (10 × GOMAXPROCS per process).

### 4. Caching Strategy

| Layer | Speed | Location |
|-------|-------|----------|
| L1 in-process memory | <1µs | Per prefork child |
| L2 Dragonfly/Redis | ~100µs | Shared across processes |
| Database (PG, MySQL, Mongo) | 1-5ms | External service |

Use cache-aside pattern: try L1, then L2, then DB. Populate caches on miss.

### 5. Seed Data

Pre-seed 200 hot keys before the benchmark. This ensures the cache is warm and every request hits the fast path.

### 6. Warmup + Measure

Each endpoint: 3s warmup (discarded) + 5s measurement:
```
wrk -t10 -c1000 -d3s ...    # warmup (discarded)
wrk -t10 -c1000 -d5s ...    # measurement
```

The warmup stabilizes connections, caches, and Go runtime before measurement.

## Methodology

1. Multi-stage Dockerfile builds the Go binary
2. Data services (PG, MariaDB, MongoDB, Dragonfly, RustFS) start in the same Docker network
3. Service starts, health check passes
4. Functional tests verify correctness (`go test -c` → `tester -test.run=TestURL|TestFile|TestNATS|...`)
5. Hot keys seeded via POST endpoints (curl) — 200 for URL shortener, 50–200 for file storage
6. `wrk -t10 -c1000` runs sequentially for each endpoint: 3s warmup (discarded) + 5s measurement (2–8 endpoints per variant)
7. Report: Requests/sec for each endpoint (pass 2)

## Environment

| Key | Value |
|-----|-------|
| Hardware | bare-metal, Apple Silicon (10 cores @ 3GHz ARM) |
| Docker | Docker Desktop (macOS) |
| Go | 1.26.4 |
| Benchmark tool | `wrk -t10 -c1000 -d5s` |

## Benchmark Suite (`runtime/benchmarks/`)

| File | Benchmarks |
|------|-----------|
| `middleware_bench_test.go` | Middleware chain overhead (no/prometheus/bodyreader/maxbytes) |
| `json_bench_test.go` | JSON marshal/unmarshal throughput |
| `crud_bench_test.go` | CRUD struct serialization |
| `cache_bench_test.go` | In-memory cache get/set |
| `nats_bench_test.go` | NATS publish/subscribe (requires NATS) |
| `http_bench_test.go` | HTTP serialization (requires server) |

Run: `make bench` or `go test -bench=. -benchmem ./runtime/benchmarks/...`

## Benchstat CI (Regression Detection)

Pull requests automatically run benchmark comparisons:

```yaml
# .github/workflows/benchmark.yml
# On PR: benchmark PR changes, checkout main, benchmark main
# benchstat comparison, fail on >5% regression
# Comment PR with results
```

Manual comparison: `make bench-compare`
Requires: `go install golang.org/x/perf/cmd/benchstat@latest`

## Profile-Guided Optimization (PGO)

The project includes a PGO marker at `cmd/sdk-api/default.pgo`:

```bash
# Verify PGO is enabled
make pgo-verify

# Collect production profile
curl -o default.pgo 'http://localhost:6060/debug/pprof/profile?seconds=30'
cp default.pgo cmd/sdk-api/default.pgo
```

PGO provides 2-7% throughput improvement at zero code cost.

## Escape Analysis CI

The CI workflow includes escape analysis to detect unintended heap allocations:

```bash
go build -gcflags='-m=2' ./... 2>&1 | grep "escapes to heap"
```

This runs as part of the `lint` job in `.github/workflows/ci.yml`.
