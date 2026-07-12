# 200-url-shortener-mongo

URL shortener with MongoDB direct.

## Quick Start

```bash
docker compose run --rm bench               # functional tests
docker compose run --rm bench --rps         # functional + RPS
```

## Benchmark (wrk -t10 -c1000 inside Docker)

| Endpoint | RPS | Notes |
|----------|:---:|-------|
| Expand (GET /expand/:shortCode) | 33,235 | MongoDB direct |
| List (GET /links) | 24,839 | Pagination with COUNT(*) |
| GetByID (GET /links/:id) | 31,066 | Direct read by PK |
| Create (POST /links) | 31,376 | Insert via MongoDB |
| Update (PUT /links/:id) | 201,035 | Update via MongoDB |
| Delete (DELETE /links/:id) | 37,922 | Delete via MongoDB |

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
