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

## Benchmark (wrk -t10 -c1000 -d30s)

| Endpoint | RPS | ±5% | ±10% |
|----------|:---:|:---:|:----:|
| Expand (GET /expand/:shortCode) | 107,056 | 101,703–112,409 | 96,350–117,762 |
| List (GET /links) | 24,274 | 23,060–25,488 | 21,847–26,701 |
| GetByID (GET /links/:id) | 45,983 | 43,684–48,282 | 41,385–50,581 |
| Create (POST /links) | 18,914 | 17,968–19,860 | 17,023–20,805 |
| Update (PUT /links/:id) | 104,815 | 99,574–110,056 | 94,334–115,297 |
| Delete (DELETE /links/:id) | 44,300 | 42,085–46,515 | 39,870–48,730 |

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
