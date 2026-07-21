# 200-url-shortener-postgres-dragonfly

URL shortener with PostgreSQL + Dragonfly cache.

## Quick Start

```bash
docker compose run --rm bench               # functional tests
docker compose run --rm bench --rps         # functional + RPS
```

## Benchmark (wrk -t10 -c1000 inside Docker)

| Endpoint | RPS | Notes |
|----------|:---:|-------|
| Expand (GET /expand/:shortCode) | 109,924 | PostgreSQL + Dragonfly L2 cache |
| List (GET /links) | 23,680 | Pagination with COUNT(*) |
| GetByID (GET /links/:id) | 35,184 | Direct read by PK |
| Create (POST /links) | 19,350 | Insert via PostgreSQL |
| Update (PUT /links/:id) | 206,590 | Update via PostgreSQL |
| Delete (DELETE /links/:id) | 35,018 | Delete via PostgreSQL |

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
