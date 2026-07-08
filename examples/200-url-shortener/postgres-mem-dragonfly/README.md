# 200-url-shortener-L1

URL shortener with dual-layer cache: L1 in-process memory (`collection.Cache`) + L2 Dragonfly. Cache-aside via `runtime.CachedCRUD`. Prefork on. Uses SDK `type: crud` — no Fiber import in user code.

**Stack:** Fiber + pgx + `collection.Cache` L1 + Dragonfly L2 + PgDog pooler.

## Configuration

| Variable | Value | Description |
|----------|-------|-------------|
| `DATABASE_URL` | `postgres://dev:devpass@pgdog:6432/postgres` | PostgreSQL via PgDog |
| `DRAGONFLY_ADDR` | `dragonfly:6379` | Dragonfly in `cluster_mode=emulated` |
| L1 TTL | 30 s | In-process memory cache |
| L2 TTL | 5 min | Dragonfly cache expiry |

## Quick Start

```bash
docker compose up --abort-on-container-exit
```

## Benchmark (wrk -t10 -c1000 -d30s, 3 runs each endpoint)

All 6 endpoints are benchmarked sequentially within the same container (200 pre-seeded keys). Expand uses L1+L2 cache.

| Endpoint | Run 1 | Run 2 | Run 3 | Average |
|----------|:-----:|:-----:|:-----:|:-----:|
| Expand (GET /expand/:shortCode) | 100,760 | 110,617 | 109,791 | **107,056** |
| List (GET /links) | 24,151 | 24,244 | 24,428 | **24,274** |
| GetByID (GET /links/:id) | 45,318 | 45,972 | 46,659 | **45,983** |
| Create (POST /links) | 19,096 | 18,918 | 18,728 | **18,914** |
| Update (PUT /links/:id) | 107,225 | 100,732 | 106,489 | **104,815** |
| Delete (DELETE /links/:id) | 44,395 | 44,573 | 43,934 | **44,300** |

## Architecture

| File | Purpose |
|------|---------|
| `models/link.go` | Link model (primary key: `id`) |
| `models/link_expand.go` | LinkExpand model (primary key: `short_code`) |
| `hooks.go` | `BeforeCreate` auto-generates short codes |
| `main.go` | `MustRegister` + `CachedCRUD` with L1+L2 |
| `service.docker.yaml` | Docker config (prefork, pool) |
| `bench_test.go` | Functional tests + BenchmarkExpand |
| `run.sh` | Entrypoint: functional tests always, RPS benchmark only with `RPS_BENCH=1` (6 endpoints) |
| `docker-compose.yml` | PostgreSQL 18 + Dragonfly + PgDog |
