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
| Expand (GET /expand/:shortCode) | 32,814 | MongoDB direct |
| List (GET /links) | 24,707 | Pagination with skip/limit |
| GetByID (GET /links/:id) | 29,321 | Direct read by PK |
| Create (POST /links) | 31,285 | Insert via MongoDB |
| Update (PATCH /links/:id) | 34,977 | Update via MongoDB |
| Delete (DELETE /links/:id) | 41,937 | Delete via MongoDB |

Measured 2026-08-01. The previous Update row (139,859) was invalid: `update.lua` used `PUT` against a PATCH-only route, returning 405 without touching the database (see docs/benchmarks.md rules 10-11).

## Architecture

| File | Purpose |
|------|---------|
| `cmd/main.go` | Bootstrap — MongoMustRegister × 2 |
| `service.yaml` | Service config (api_prefix: /api) |
| `service.docker.yaml` | Docker config override |
| `run.sh` | Entrypoint: --rps for benchmarks, --test:Name for specific tests |
| `bench_test.go` | Functional tests + expand benchmark |
| `docker-compose.yml` | Services definition |
