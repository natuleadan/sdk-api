# 200-url-shortener-postgres-dragonfly

URL shortener with PostgreSQL + Dragonfly cache.

## Quick Start

```bash
docker compose run --rm bench               # functional tests
docker compose run --rm bench --rps         # functional + RPS
```

## Benchmark (wrk -t10 -c1000 inside Docker)

| Endpoint | Dedicated (12-core) | Local (10-core) | Baseline |
|----------|:---:|:---:|:---:|
| Expand (GET /expand/:shortCode) | 157,247 | 119,304 | 123,869 |
| List (GET /links) | 30,194 | 20,575 | 21,334 |
| GetByID (GET /links/:id) | 61,270 | 42,327 | 43,123 |
| Create (POST /links) | 27,664 | 22,706 | 19,553 |
| Update (PATCH /links/:id) | 25,488 | 23,700 | 17,192 |
| Delete (DELETE /links/:id) | 52,438 | 28,774 | 35,239 |

Measured 2026-08-19 on v0.18.2 (Dedicated = 12-core AMD Linux box; Local =
10-core ARM macOS; wrk inside Docker, clean host before measuring; PgDog pool
200/50/8 + Dragonfly `--proactor_threads=2 --maxclients=20000`). Baseline
2026-08-01.

## Architecture

| File | Purpose |
|------|---------|
| `cmd/main.go` | Bootstrap — MustRegister + CachedCRUD (write-through) |
| `models/link.go` | Link model + BeforeCreate hook |
| `models/link_expand.go` | LinkExpand model (PK: short_code, cached) |
| `service.yaml` | Service config (api_prefix: /api) |
| `service.docker.yaml` | Docker config (prefork, pool, PgDog) |
| `run.sh` | Entrypoint: --rps for benchmarks, --test:Name for specific tests |
| `bench_test.go` | Functional tests + expand benchmark |
| `docker-compose.yml` | PostgreSQL 18 + PgDog + Dragonfly |
