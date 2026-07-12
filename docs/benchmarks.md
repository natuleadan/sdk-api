# Benchmarks

How to measure and maximize RPS with the sdk-api framework.

## Rules

1. **All benchmarks run fully inside Docker.** Running the Go binary on the host while data services run in Docker adds 2-4x latency due to Docker Desktop port mapping. The `wrk` tool runs inside the same container as the service — never on the macOS host.
2. **Use wrk, not Go testing.B** for high-concurrency benchmarks. Go goroutines have scheduling overhead at 1000+ concurrency.
3. **Each folder uses `docker compose run`** via `run.sh` which accepts:
   - `--rps`: run functional tests then RPS benchmarks (wrk inside container, 3s warmup + 5s measure)
   - `TestName`: run a specific test (e.g., `./run.sh TestHealthz_OK`)
   - `--test:Name`: same as `TestName` (e.g., `--test:TestHealthz_OK`)
4. **PostgreSQL max_connections must match pool size.** Use `command: ["postgres", "-c", "max_connections=200"]` with PgDog managing the pool.
5. **Results are maintained in each example's README.** Re-run the benchmark to verify. Update the README if the result changes.
6. **Functional tests run by default** (no flags needed). The container entrypoint (`run.sh`) runs the test binary for that variant.
7. **Each example seeds hot keys** before the RPS benchmark (via curl POST). 200 for URL shortener, 50 for pg-nats, etc. This ensures caches are warm and every request hits the fast path.
8. **Endpoints are measured sequentially** — 2–8 depending on the variant. Each endpoint gets 30s warmup + 30s measurement.
9. **wrk runs INSIDE the container, not on macOS host.** Running wrk from macOS against a Docker container adds virtualisation overhead and produces invalid RPS numbers. Use `--rps` (not `--local --rps`) for official benchmarks.

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
- PgDog pool size: `20`. PostgreSQL `max_connections`: `200`.
- Without a pooler, set reasonable `max_conns` on the application pool.

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
2. Data services (PG, Redis, MariaDB, MongoDB, Dragonfly) start in the same Docker network
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
