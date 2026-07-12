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
| Expand (GET /expand/:shortCode) | 93,423 | PostgreSQL + Dragonfly cache |
| List (GET /links) | 25,305 | Pagination with COUNT(*) |
| GetByID (GET /links/:id) | 48,434 | Direct read by PK |
| Create (POST /links) | 19,363 | Insert via PostgreSQL |
| Update (PUT /links/:id) | 233,535 | Update via PostgreSQL |
| Delete (DELETE /links/:id) | 41,975 | Delete via PostgreSQL |

## Architecture

| File | Purpose |
|------|---------|
| `main.go` |  |
| `hooks.go` | BeforeCreate auto-generates short codes |
| `models/link.go` | Link model (PK: id) |
| `models/link_expand.go` | LinkExpand model (PK: short_code) |
| `service.docker.yaml` | Docker config |
| `run.sh` | Entrypoint: --rps for benchmarks, --test:Name for specific tests |
| `bench_test.go` | Functional tests + expand benchmark |
| `docker-compose.yml` | Services definition |
