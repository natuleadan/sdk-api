# Benchmarks

How to measure and maximize RPS with the sdk-api framework.

## Rules

1. **All benchmarks run fully inside Docker.** Running the Go binary on the host while data services run in Docker adds 2-4x latency due to Docker Desktop port mapping.
2. **Use wrk, not Go testing.B** for high-concurrency benchmarks. Go goroutines have scheduling overhead at 1000+ concurrency.
3. **Each folder is self-contained.** `docker compose up --abort-on-container-exit` builds, seeds, and runs.
4. **PostgreSQL max_connections must match pool size.** Use `command: ["postgres", "-c", "max_connections=200"]` with PgDog managing the pool.
5. **Results are maintained in each example's README.** Re-run `docker compose up --abort-on-container-exit` to verify. Update the README if the result changes.
6. **Functional tests run by default.** The container entrypoint (`run.sh`) always runs `tester -test.run=TestURL`. To also run the RPS benchmark:
   ```bash
   RPS_BENCH=1 docker compose up --abort-on-container-exit
   ```
7. **Each example seeds 200 hot keys** before the RPS benchmark (via curl POST). This ensures caches are warm and every request hits the fast path.
8. **Six endpoints are measured sequentially** per `run.sh`: expand → list → getbyid → create → update → delete. Each endpoint gets 30s warmup + 30s measurement.

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

Pre-seed 200 hot keys before the benchmark. This ensures the cache is warm and every request hits the fast path.

### 6. Warmup

Each endpoint runs 30s warmup before 30s measurement:
```
wrk -t10 -c1000 -d30s ...    # warmup (populates caches)
wrk -t10 -c1000 -d30s ...    # measurement
```

Report the second pass.

## Methodology

1. Multi-stage Dockerfile builds the Go binary
2. Data services (PG, Redis, MariaDB, MongoDB, Dragonfly) start in the same Docker network
3. Service starts, health check passes
4. Functional tests verify correctness (`go test -c` → `tester -test.run=TestURL`)
5. 200 records seeded via POST endpoints (curl)
6. `wrk -t10 -c1000 -d30s` runs sequentially for each of the 6 endpoints (expand, list, getbyid, create, update, delete) — each with warmup + measure
7. Report: Requests/sec for each endpoint (pass 2)

## Environment

| Key | Value |
|-----|-------|
| Hardware | Bare Metal, Apple Silicon (10 cores @ 3GHz ARM) |
| Docker | Docker Desktop (macOS) |
| Go | 1.26.4 |
| Benchmark tool | `wrk -t10 -c1000 -d30s` |
