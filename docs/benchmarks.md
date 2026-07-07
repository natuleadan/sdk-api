# Benchmarks

How to measure and maximize RPS with the sdk-api framework.

## Rules

1. **All benchmarks run fully inside Docker.** Running the Go binary on the host while data services run in Docker adds 2-4x latency due to Docker Desktop port mapping.
2. **Use wrk, not Go testing.B** for high-concurrency benchmarks. Go goroutines have scheduling overhead at 1000+ concurrency.
3. **Each folder is self-contained.** `docker compose up --abort-on-container-exit` builds, seeds, and runs.
4. **PostgreSQL max_connections must match pool size.** Use `command: ["postgres", "-c", "max_connections=200"]` with PgDog managing the pool.
5. **Services using runtime.New must lazy-init the DB pool.** `svc.Pool()` returns nil before `svc.Run()`. Use `sync.Once` or `WithCRUDFactory` to defer pool access.
6. **Results are maintained in each example's README.** Run `docker compose up --abort-on-container-exit` and verify RPS matches the documented value. Update the README if the result changes.
7. **Each example uses `run.sh`** as the Docker CMD: it runs functional tests (via `go test -c`), seeds data (100 POST requests), then executes `wrk -t10 -c1000 -d30s`. Cache variants run 2 wrk passes (warmup + measure).

## Maximizing RPS

### 1. Prefork

Enable `prefork: true` for multi-core throughput when the bottleneck is CPU-bound (middleware chain, JSON serialization, cache hits).
When the bottleneck is the database, prefork does not improve throughput — all processes compete for the same DB connections.

### 2. Middleware

The standard middleware stack (logger, shedding, breaker, maxconns, maxbytes, gunzip, prometheus) has minimal overhead on simple endpoints. For maximum throughput:

```yaml
server:
  middleware:
    - path: "/api/v1/*"
      apply: []
```

This disables the 8 standard middlewares per-route. Four middlewares are always active (recover, header sanitize, health endpoint, metrics endpoint) with negligible overhead.

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

Pre-seed 100+ hot keys before the benchmark. This ensures the cache is warm and every request hits the fast path.

### 6. Warmup

Run two wrk passes:
```
wrk -t10 -c1000 -d30s ...    # pass 1: warmup (populates caches)
wrk -t10 -c1000 -d30s ...    # pass 2: measurement
```

The first pass warms caches across all prefork children. Report the second pass.

## Methodology

1. Multi-stage Dockerfile builds the Go binary
2. Data services (PG, Redis, MariaDB, MongoDB, Dragonfly) start in the same Docker network
3. Service starts, health check passes
4. Functional tests verify correctness
5. 100 records seeded via POST endpoints
6. `wrk -t10 -c1000 -d30s` runs against the read endpoint
7. Report: Requests/sec (pass 2)

## Environment

| Key | Value |
|-----|-------|
| Hardware | Bare Metal, 10 cores |
| Docker | Docker Desktop (macOS) |
| Go | 1.26.4 |
| Benchmark tool | `wrk -t10 -c1000 -d30s` |
