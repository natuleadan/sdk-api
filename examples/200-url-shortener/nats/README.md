# 200-url-shortener-nats

URL shortener with PostgreSQL, NATS JetStream events, and NATS KV cache-aside. CRUD operations publish events (created/updated/deleted/expanded) to JetStream. Reads use NATS KV as a cache-aside layer (keys `sc.` for expand, `id.` for get-by-id), invalidated inline on writes — no stale window. Uses SDK `type: crud` with a single override for the cache-enabled Get handler.

**Stack:** Fiber + pgx + PgDog pooler + NATS JetStream + NATS KV + Core NATS.

## Configuration

| Variable | Value | Description |
|----------|-------|-------------|
| `DATABASE_URL` | `postgres://dev:devpass@pgdog:6432/postgres` | PostgreSQL via PgDog |
| `NATS_URL` | `nats://nats:4222` | NATS with JetStream enabled |
| `CONFIG_PATH` | `service.docker.yaml` | Prefork off, pool 500 |
| KV bucket | `url-expand-cache` | Memory storage, 5 min TTL fallback |

## Quick Start

```bash
docker compose run --rm bench               # functional tests
docker compose run --rm bench --rps         # functional + RPS
```

## Benchmark (wrk -t10 -c1000 inside Docker)

| Endpoint | RPS | Notes |
|----------|:---:|-------|
| List (GET /links) | 18,714 | PG scan + pagination |
| Expand (GET /expand/:shortCode) | 109,972 | Cache-aside (NATS KV) |
| Create (POST /links) | 16,672 | PG insert + cache invalidation + event publish |
| Update (PATCH /links/:id) | 13,377 | PG update + cache invalidation + event publish |
| Delete (DELETE /links/:id) | 32,874 | PG delete + cache invalidation + event publish |
| RPC (POST /nats/rpc) | 129,005 | Core NATS request-reply |
| KV Get (GET /nats/kv/:key) | 86,891 | NATS KV standalone read |
| KV Set (PUT /nats/kv/:key) | 62,327 | NATS KV standalone write |

Measured 2026-08-01 with PgDog pool 200/50/8 + max_connections 500 (fix applied 2026-07-31). Create recovered from 2,149 (untuned PgDog pool 20) to ~16-18K (+13% vs 14,908 baseline). Update with `math.random(1,200)` was throttled to ~9K by row-lock contention on 200 hot rows; widening the range to 1-2000 (docs/benchmarks.md rule 11) restored it to 13,377, on par with the 14,192 baseline.

## Architecture

| File | Purpose |
|------|---------|
| `cmd/main.go` | Bootstrap — MustRegister + handler routes + exit workers |
| `models/link.go` | Link model + URLEvent + lifecycle hooks (cache invalidation + event publish) |
| `internal/handler/links.go` | CRUD override (cache-aside) + expand with event publish |
| `internal/handler/nats.go` | NATS RPC, KV, pull handlers |
| `internal/handler/admin.go` | Events admin handlers |
| `internal/svc/servicecontext.go` | DI container — event broker, cache conn, exit workers |
| `service.docker.yaml` | Docker config (prefork off, pool 500, PgDog) |
| `bench_test.go` | 16 functional tests including cache invalidation + worker bulk (358k/s) |
| `run.sh` | Entrypoint: `--rps` for benchmarks, `--test:Name` for specific tests |
| `docker-compose.yml` | PostgreSQL 18 + PgDog + NATS JetStream |
