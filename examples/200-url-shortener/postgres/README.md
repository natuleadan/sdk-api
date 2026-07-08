# 200-url-shortener-postgres

URL shortener with PostgreSQL and PgDog pooler. No cache — every request hits the database. Two CRUD entries on the same `link` table: one with `id` as PK, another with `short_code` as PK for the expand endpoint. Uses SDK `type: crud` — no Fiber import in user code.

**Stack:** Fiber + pgx + PgDog pooler.

## Configuration

| Variable | Value | Description |
|----------|-------|-------------|
| `DATABASE_URL` | `postgres://dev:devpass@pgdog:6432/postgres` | PostgreSQL via PgDog |
| `CONFIG_PATH` | `service.docker.yaml` | Prefork on, pool 500 |
| PgDog pool | `20` | Transaction pooler, no `[admin]` |

## Quick Start

```bash
docker compose up --abort-on-container-exit
```

## Benchmark (wrk -t10 -c1000 -d30s, 3 runs each endpoint)

All 6 endpoints are benchmarked sequentially within the same container (200 pre-seeded keys). Order: expand → list → getbyid → create → update → delete.

| Endpoint | Run 1 | Run 2 | Run 3 | Average |
|----------|:-----:|:-----:|:-----:|:-----:|
| Expand (GET /expand/:shortCode) | 45,214 | 44,400 | 43,775 | **44,463** |
| List (GET /links) | 22,899 | 23,560 | 22,754 | **23,071** |
| GetByID (GET /links/:id) | 40,948 | 46,513 | 44,570 | **44,010** |
| Create (POST /links) | 16,674 | 15,895 | 16,812 | **16,460** |
| Update (PUT /links/:id) | 98,347 | 93,405 | 95,378 | **95,710** |
| Delete (DELETE /links/:id) | 39,602 | 42,372 | 44,390 | **42,121** |

## Architecture

| File | Purpose |
|------|---------|
| `models/link.go` | Link model (primary key: `id`) |
| `models/link_expand.go` | LinkExpand model (primary key: `short_code`) |
| `hooks.go` | `BeforeCreate` auto-generates short codes |
| `main.go` | Bootstrap via `runtime.MustRegister` |
| `service.docker.yaml` | Docker config (prefork, pool, PgDog) |
| `bench_test.go` | Functional tests + Go benchmark |
| `run.sh` | Entrypoint: functional tests always, RPS benchmark only with `RPS_BENCH=1` (6 endpoints) |
| `docker-compose.yml` | PostgreSQL 18 + PgDog |
