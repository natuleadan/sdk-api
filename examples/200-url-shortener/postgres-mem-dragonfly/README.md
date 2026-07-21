# 200-url-shortener-postgres-mem-dragonfly

URL shortener with PostgreSQL + RAM L1 + Dragonfly L2.

## Quick Start

```bash
docker compose run --rm bench               # functional tests
docker compose run --rm bench --rps         # functional + RPS
```

## Benchmark (wrk -t10 -c1000 inside Docker)

| Endpoint | RPS | Notes |
|----------|:---:|-------|
| Expand (GET /expand/:shortCode) | 95,809 | PostgreSQL + RAM L1 (sync.Map) + Dragonfly L2 |
| List (GET /links) | 22,796 | Pagination with COUNT(*) |
| GetByID (GET /links/:id) | 34,935 | Direct read by PK |
| Create (POST /links) | 18,374 | Insert via PostgreSQL |
| Update (PUT /links/:id) | 180,258 | Update via PostgreSQL |
| Delete (DELETE /links/:id) | 33,857 | Delete via PostgreSQL |

## Architecture

| File | Purpose |
|------|---------|
| `cmd/main.go` | Bootstrap — MustRegister + CachedCRUD with L1+L2 |
| `models/link.go` | Link model + BeforeCreate hook |
| `models/link_expand.go` | LinkExpand model (PK: short_code, L1+L2 cached) |
| `service.yaml` | Service config (api_prefix: /api) |
| `service.docker.yaml` | Docker config (prefork, pool, PgDog) |
| `run.sh` | Entrypoint: --rps for benchmarks, --test:Name for specific tests |
| `bench_test.go` | Functional tests + expand benchmark |
| `docker-compose.yml` | PostgreSQL 18 + PgDog + Dragonfly |
