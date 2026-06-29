# Benchmarks

> Benchmarks measured with **v0.1.0** on the hardware below.

## Rules

1. **Cumulative record.** No result is ever deleted or modified retroactively.
2. **All benchmarks run fully inside Docker.** Running the Go binary on the host while data services run in Docker adds 2-4x latency due to Docker Desktop port mapping.
3. **Use wrk, not Go testing.B** for high-concurrency benchmarks. Go goroutines have scheduling overhead at 1000+ concurrency.
4. **Each folder is self-contained.** `docker compose up --abort-on-container-exit` builds, seeds, and runs.
5. **Target: 30k RPS** (matching go-zero's shorturl benchmark on Bare Metal).

## Environment

| Key | Value |
|-----|-------|
| Hardware | Bare Metal, 10 cores |
| Docker | Docker Desktop (macOS) |
| Go | 1.26.4 |
| Benchmark tool | `wrk -t10 -c1000 -d30s` |

## Results

### healthz — Minimal HTTP throughput

`GET /healthz → 200 OK`. No DB, no NATS, no business logic.

| Run | Date | RPS | Latency p50 | Notes |
|-----|------|-----|-------------|-------|
| 1 | 2026-06-27 | 87,400 | — | Go test (10 workers) |
| 2 | 2026-06-27 | 696,350 | 1.08ms | dockerized wrk |
| 3 | 2026-06-27 | 748,826 | 0.87ms | dockerized wrk (retry) |
| 4 | 2026-06-28 | 680,867 | — | RAW Fiber (Arm Bare Metal 10c, native arm64) |
| 5 | 2026-06-28 | 672,302 | — | SDK full /healthz (Mac, short-circuit) |
| 6 | 2026-06-28 | 148,696 | — | SDK full /ping — **14 mw actual** |
| 7 | 2026-06-28 | 689,418 | — | SDK minimal /ping — recover+health only |

### healthz — Cross-environment (2026-06-28)

`wrk -t10 -c1000 -d30s` (Mac, VPS A) · `wrk -t2 -c200 -d30s` (VPS B 1c)

| Mode | Arm Bare Metal 10c | 🟢 VPS A (4c dedicated) | 🔵 VPS B (1c shared) |
|------|:-------------:|:----------------------:|:--------------------:|
| RAW Fiber (/healthz) | 680,867 | 108,184 | 56,170 |
| SDK full /healthz | 672,302 | 110,346 | 55,529 |
| SDK full /ping (14 mw) | **148,696** | **32,153** | **6,571** |
| SDK minimal /ping | **689,418** | **103,697** | **55,934** |

### url-link-base — Redis cache

`GET /api/v1/expand/:code`. Cache: Redis. Prefork: false (Redis connection is per-process).

| Run | Date | RPS | Latency p50 | Errors |
|-----|------|-----|-------------|--------|
| 1 | 2026-06-27 | 64,750 | 15.42ms | 0% |

### url-link-nats — NATS KV cache

`GET /api/v1/expand/:code`. Cache: NATS KV (MemoryStorage). Prefork: true (NATS KV is server-side shared).

| Run | Date | RPS | Latency p50 | Errors |
|-----|------|-----|-------------|--------|
| 1 | 2026-06-27 | 80,297 | 12.90ms | 0% |

### mysql — MySQL CRUD

`GET /api/v1/product/:id`. DB: MySQL 8.0 via `db.MySQLTable[T]`. Prefork: true.

| Run | Date | RPS | Latency p50 | Errors |
|-----|------|-----|-------------|--------|

### turso — Turso/SQLite CRUD

`GET /api/v1/product/:id`. DB: Turso embebido via `db.TursoTable[T]`. Prefork: true.

| Run | Date | RPS | Latency p50 | Errors |
|-----|------|-----|-------------|--------|

### mongo — MongoDB CRUD

`GET /api/v1/product/:id`. DB: MongoDB 7 via `infra/stores/mon`. Prefork: true.

| Run | Date | RPS | Latency p50 | Errors |
|-----|------|-----|-------------|--------|

### Comparison vs go-zero shorturl

| Metric | go-zero shorturl | sdk-api url-link-nats | sdk-api url-link-base |
|--------|-----------------|-------------------|-------------------|
| **RPS** | **33,024** | **80,297** (2.4x) | **64,750** (1.96x) |
| Cache | Redis | NATS KV | Redis |
| Middleware | ~12 | 14 | 14 |
| Environment | Linux bare metal | macOS Docker Desktop | macOS Docker Desktop |
| Concurrency | wrk -t10 -c1000 | wrk -t10 -c1000 | wrk -t10 -c1000 |

## Running a Benchmark

```bash
# HTTP throughput
cd examples/healthz
docker compose up --abort-on-container-exit

# Redis cache
cd examples/url-link-base
docker compose up --abort-on-container-exit

# NATS KV cache (+ PG)
cd examples/url-link-nats
docker compose up --abort-on-container-exit

# MySQL CRUD
cd examples/mysql
docker compose up --abort-on-container-exit

# Turso/SQLite CRUD
cd examples/turso
docker compose up --abort-on-container-exit

# MongoDB CRUD
cd examples/mongo
docker compose up --abort-on-container-exit
```

All benchmarks are self-contained. Each folder has its own `docker-compose.yml`, `Dockerfile`, and `run.sh`.

## Methodology

1. Build the Go binary via multi-stage Dockerfile
2. Start data services (PG, Redis, NATS, MySQL, MongoDB) in same Docker network
3. Seed 100 records with predefined IDs
4. Run `wrk -t10 -c1000 -d30s` against the read endpoint
5. Report: Requests/sec, latency distribution, error rate

Historical results are recorded in `docs/benchmarks_history.md`.
