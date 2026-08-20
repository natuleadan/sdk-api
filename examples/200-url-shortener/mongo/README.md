# 200-url-shortener-mongo

URL shortener with MongoDB direct.

## Quick Start

```bash
docker compose run --rm bench               # functional tests
docker compose run --rm bench --rps         # functional + RPS
```

## Benchmark (wrk -t10 -c1000 inside Docker)

| Endpoint | Dedicated (12-core) | Local (10-core) | Baseline |
|----------|:---:|:---:|:---:|
| Expand (GET /expand/:shortCode) | 57,771 | 33,250 | 32,814 |
| List (GET /links) | 145,331 | 25,064 | 24,707 |
| GetByID (GET /links/:id) | 157,453 | 29,269 | 29,321 |
| Create (POST /links) | 149,272 | 34,117 | 31,285 |
| Update (PATCH /links/:id) | 127,218 | 34,491 | 34,977 |
| Delete (DELETE /links/:id) | 144,655 | 39,905 | 41,937 |

> [!tip] Fix 2026-08-19: index on lookup field
> The dedicated expand measured 199 RPS (COLLSCAN) — `MongoMustRegister` did not
> create an index on the custom lookup field (`shortCode`). Fixed by
> `mon.Model.EnsureIndex` + a call in register: expand 199 → **57,771** on the
> dedicated box. The other endpoints (127-157K) were already indexed or inserts.

Measured 2026-08-19 on v0.18.2 (Dedicated = 12-core AMD Linux box; Local =
10-core ARM macOS; wrk inside Docker, clean host before measuring). Baseline
2026-08-01. The previous Update row (139,859) was invalid: `update.lua` used
`PUT` against a PATCH-only route (see docs/benchmarks.md rules 10-11).

## Architecture

| File | Purpose |
|------|---------|
| `cmd/main.go` | Bootstrap — MongoMustRegister × 2 |
| `service.yaml` | Service config (api_prefix: /api) |
| `service.docker.yaml` | Docker config override |
| `run.sh` | Entrypoint: --rps for benchmarks, --test:Name for specific tests |
| `bench_test.go` | Functional tests + expand benchmark |
| `docker-compose.yml` | Services definition |
