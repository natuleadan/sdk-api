# 200-url-shortener-pg-dragonfly

URL shortener with PostgreSQL and Dragonfly cache (L2). Cache-aside via `runtime.CachedCRUD`. Prefork on. Uses SDK `type: crud` — no Fiber import in user code.

**Stack:** Fiber + pgx + Dragonfly + PgDog pooler.

## Configuration

| Variable | Value | Description |
|----------|-------|-------------|
| `DATABASE_URL` | `postgres://dev:devpass@pgdog:6432/postgres` | PostgreSQL via PgDog |
| `DRAGONFLY_ADDR` | `dragonfly:6379` | Dragonfly in `cluster_mode=emulated` |
| L2 TTL | 5 min | Dragonfly cache expiry |

## Quick Start

```bash
docker compose up --abort-on-container-exit
```

## Benchmark (wrk -t10 -c1000 -d30s, 3 runs each endpoint)

All 6 endpoints are benchmarked sequentially within the same container (200 pre-seeded keys). Expand uses L2 Dragonfly cache.

| Endpoint | Run 1 | Run 2 | Run 3 | Average |
|----------|:-----:|:-----:|:-----:|:-----:|
| Expand (GET /expand/:shortCode) | 76,626 | 76,065 | 72,798 | **75,163** |
| List (GET /links) | 24,352 | 24,366 | 24,156 | **24,291** |
| GetByID (GET /links/:id) | 46,856 | 46,734 | 45,461 | **46,350** |
| Create (POST /links) | 19,037 | 19,399 | 19,185 | **19,207** |
| Update (PUT /links/:id) | 105,269 | 110,268 | 106,848 | **107,461** |
| Delete (DELETE /links/:id) | 45,126 | 45,317 | 45,286 | **45,243** |

## Architecture

| File | Purpose |
|------|---------|
| `models/link.go` | Link model (primary key: `id`) |
| `models/link_expand.go` | LinkExpand model (primary key: `short_code`) |
| `hooks.go` | `BeforeCreate` auto-generates short codes |
| `main.go` | `MustRegister` + `CachedCRUD` with Dragonfly |
| `service.docker.yaml` | Docker config (prefork, pool) |
| `bench_test.go` | Functional tests + BenchmarkExpand |
| `run.sh` | Entrypoint: functional tests always, RPS benchmark only with `RPS_BENCH=1` (6 endpoints) |
| `docker-compose.yml` | PostgreSQL 18 + Dragonfly + PgDog |
