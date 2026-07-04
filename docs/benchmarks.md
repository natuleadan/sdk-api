# Benchmarks

> Benchmarks measured with **v0.3.0** on the hardware below.

## Rules

1. **Cumulative record.** No result is ever deleted or modified retroactively.
2. **All benchmarks run fully inside Docker.** Running the Go binary on the host while data services run in Docker adds 2-4x latency due to Docker Desktop port mapping.
3. **Use wrk, not Go testing.B** for high-concurrency benchmarks. Go goroutines have scheduling overhead at 1000+ concurrency.
4. **Each folder is self-contained.** `docker compose up --abort-on-container-exit` builds, seeds, and runs.
5. **Target: 30k RPS** (matching go-zero's shorturl benchmark on Bare Metal).
6. **PostgreSQL max_connections must match pool size.** Docker Compose sets `command: ["postgres", "-c", "max_connections=1000"]` for benchmarks with wrk -c1000.
7. **Services using runtime.New must lazy-init the DB pool.** `svc.Pool()` returns nil before `svc.Run()`. Use `sync.Once` to defer pool access until the first HTTP request.

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
| **8** | **2026-07-03** | **741,769** | **1.52ms** | RAW Fiber (Go 1.26) |
| **9** | **2026-07-03** | **752,163** | **1.03ms** | SDK full /healthz (Go 1.26) |

Cross-environment results (2026-06-28) omitted for brevity. See `benchmarks_history.md`.

### url-link-base — Redis cache

`GET /api/v1/expand/:code`. Cache: Redis. Prefork: true. PgDog connection pooler.

| Run | Date | RPS | Latency p50 | Errors | Notes |
|-----|------|-----|-------------|--------|-------|
| 1 | 2026-06-27 | 64,750 | 15.42ms | 0% | |
| 2 | 2026-07-03 | **99,278** | **10.57ms** | 0% | Lazy pool init + PG mc=500 |
| **3** | **2026-07-04** | **151,091** | **—** | **0%** | **+PgDog pooler** |

### url-link-nats — NATS KV cache

`GET /api/v1/expand/:code`. Cache: NATS KV (MemoryStorage). Prefork: true. PgDog connection pooler.

| Run | Date | RPS | Latency p50 | Errors | Notes |
|-----|------|-----|-------------|--------|-------|
| 1 | 2026-06-27 | 80,297 | 12.90ms | 0% | |
| 2 | 2026-07-03 | 67,283 | — | 0% | Lazy pool init + PG mc=1000 |
| **3** | **2026-07-04** | **141,162** | **—** | **0%** | **+PgDog pooler + event_streams** |

### auth-none-monolith — Post CRUD (driver: none)

`POST /api/v1/posts` (INSERT) · `GET /api/v1/debug/items` (SELECT LIMIT 100). Driver: none. No auth middleware executes. Pool: 1000. PgDog pooler.

| Endpoint | Date | RPS | Latency p50 | Errors | Notes |
|----------|------|-----|-------------|--------|-------|
| POST | 2026-07-04 | **29,406** | — | 0.5% | No cache — PG per request |
| GET | 2026-07-04 | **28,447** | — | 0% | No cache — PG per request |

### auth-none-microservices — Users & Products CRUD (driver: none)

Two independent services sharing one PostgreSQL. Each pool: 500. PgDog pooler.

| Service | Date | RPS | Latency p50 | Errors | Notes |
|---------|------|-----|-------------|--------|-------|
| Users GET | 2026-07-04 | **27,973** | — | 0% | No cache — PG per request |
| Products GET | 2026-07-04 | **28,014** | — | 0% | No cache — PG per request |

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

### Cache vs no-cache: why 150k vs 30k RPS

| Pattern | RPS | Bottleneck | Examples |
|---------|:---:|------------|----------|
| **Cache hit** (Redis/NATS KV) | **140-151k** | Docker Desktop network + Go serialization | url-link-base, url-link-nats |
| **Direct PG query** | **28-29k** | PostgreSQL throughput in Docker | auth-none-monolith, auth-none-microservices |

With a cache layer (Redis or NATS KV), 99% of requests never touch PostgreSQL. The app reads from in-memory cache in microseconds. Without cache, every request executes a PG query (SELECT/INSERT), and PG becomes the bottleneck at ~30k qps in Docker Desktop.

Adding more middleware or removing auth logic does not change this — the bottleneck is PG, not Go. The healthz benchmark (no PG) confirms the SDK can sustain 700k+ RPS.

### Comparison vs go-zero shorturl

| Metric | go-zero shorturl | sdk-api url-link-nats | sdk-api url-link-base | sdk-api auth-none-monolith | sdk-api auth-none-microservices |
|--------|-----------------|-------------------|-------------------|--------------------------|-------------------------------|
| **RPS** | **33,024** | **80,297** → **67,283** | **64,750** → **99,278** | **32,252** (GET) | **41,117** (Users GET) |
| Cache | Redis | NATS KV | Redis | — | — |
| Middleware | ~12 | 14 | 14 | 14 | 14 |
| Auth driver | — | none | none | none | none |
| Pool | — | — | 500 | 1000 | 50×2 |
| Environment | Linux bare metal | macOS Docker Desktop | macOS Docker Desktop | macOS Docker Desktop | macOS Docker Desktop |
| Concurrency | wrk -t10 -c1000 | wrk -t10 -c1000 | wrk -t10 -c1000 | wrk -t10 -c1000 | wrk -t10 -c1000 |

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

# Post CRUD (no auth)
cd examples/auth-none-monolith
docker compose up --abort-on-container-exit

# Users + Products CRUD (no auth)
cd examples/auth-none-microservices
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
6. For auth examples, `driver: none` ensures zero auth middleware overhead
7. PostgreSQL deployments use **PgDog** as connection pooler to prevent connection storms from 1000-concurrent-wrk
8. PostgreSQL deployments use `command: ["postgres", "-c", "max_connections=200"]` (PgDog manages the pool size)

Historical results are recorded in `docs/benchmarks_history.md`.

Auth-related benchmarks are additive: new auth examples do not replace or modify healthz/url-link results. The auth SDK imports (`openfga`, `zitadel`, `ory`) are compiled into any binary using `runtime.New()`, but they carry zero runtime overhead — no `func init()` and dead-code eliminated by the Go linker when unused.

When comparing `auth-none-monolith` (pool=1000) with `auth-none-microservices` (pool=50×2), the monolith's single pool shares capacity across POST and GET benchmarks, while microservices allocate independent pools per wrk run.
