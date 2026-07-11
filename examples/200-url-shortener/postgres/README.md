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

## Benchmark (wrk -t10 -c1000 -d30s)

| Endpoint | RPS | ±5% | ±10% |
|----------|:---:|:---:|:----:|
| Expand (GET /expand/:shortCode) | 44,463 | 42,240–46,686 | 40,017–48,909 |
| List (GET /links) | 23,071 | 21,917–24,225 | 20,764–25,378 |
| GetByID (GET /links/:id) | 44,010 | 41,810–46,211 | 39,609–48,411 |
| Create (POST /links) | 16,460 | 15,637–17,283 | 14,814–18,106 |
| Update (PUT /links/:id) | 95,710 | 90,925–100,496 | 86,139–105,281 |
| Delete (DELETE /links/:id) | 42,121 | 40,015–44,227 | 37,909–46,333 |

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
