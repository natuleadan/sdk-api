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

## Benchmark (wrk -t10 -c1000 -d30s)

| Endpoint | RPS | ±5% | ±10% |
|----------|:---:|:---:|:----:|
| Expand (GET /expand/:shortCode) | 75,163 | 71,405–78,921 | 67,647–82,679 |
| List (GET /links) | 24,291 | 23,076–25,506 | 21,862–26,720 |
| GetByID (GET /links/:id) | 46,350 | 44,033–48,668 | 41,715–50,985 |
| Create (POST /links) | 19,207 | 18,247–20,167 | 17,286–21,128 |
| Update (PUT /links/:id) | 107,461 | 102,088–112,834 | 96,715–118,207 |
| Delete (DELETE /links/:id) | 45,243 | 42,981–47,505 | 40,719–49,767 |

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
