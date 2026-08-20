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

| Endpoint | Dedicated (12-core) | Local (10-core) | Baseline |
|----------|:---:|:---:|:---:|
| List (GET /links) | 23,649 | 24,584 | 18,714 |
| Expand (GET /expand/:shortCode) | 133,015 | 115,512 | 109,972 |
| Create (POST /links) | 35,182 | 25,056 | 16,672 |
| Update (PATCH /links/:id) | 28,962 | 26,768 | 13,377 |
| Delete (DELETE /links/:id) | 54,321 | 36,828 | 32,874 |
| RPC (POST /nats/rpc) | 154,614 | 132,957 | 129,005 |
| KV Get (GET /nats/kv/:key) | 134,279 | 118,634 | 86,891 |
| KV Set (PUT /nats/kv/:key) | 134,967 | 117,011 | 62,327 |

Measured 2026-08-19 on v0.18.2 (Dedicated = 12-core AMD Linux box; Local =
10-core ARM macOS; wrk inside Docker, clean host before measuring). Baseline
2026-08-01 with PgDog pool 200/50/8 + max_connections 500. Update with
`math.random(1,200)` was throttled to ~9K by row-lock contention on 200 hot
rows; widening the range to 1-2000 (docs/benchmarks.md rule 11) restored it to
13,377.

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

## Connecting to a secure (mTLS) cluster

The stream config supports `user`/`password`/`ca_file`/`cert_file`/`key_file`:

```yaml
stream:
  - name: primary
    driver: nats
    url: "${NATS_URL}"              # tls://user:pass@host:4222 also works for auth
    user: "${NATS_USER}"
    password: "${NATS_PASSWORD}"
    ca_file: "${NATS_CA}"
    cert_file: "${NATS_CERT}"
    key_file: "${NATS_KEY}"
```

Two validated deployment forms:

- **Remote (operator → cluster):** run the app/tests from the Mac against the public
  URLs with the operator's certs. Validated with `examples/run.sh 200/nats` (docker compose
  + the bench tests) and a focused `events.Connect` Go program.
- **Intra-VPS (microservice on the same VPS as NATS):** run the app inside the VPS,
  `NATS_URL=tls://127.0.0.1:4222` (loopback), reusing the VPS's own NATS certs at
  `/opt/sdk-ops/services/nats/certs/`. Run the tester with `DOCKER_TEST=1` (skips the
  in-test `go build`; uses the already-running app).

### KV buckets on a cluster

The default KV config is Memory + 256MB + TTL 5min (single replica), which exceeds a
small node's memory store. On a cluster, pre-create the app's buckets first:

```
nats kv add demo-kv --replicas 3 --storage file
nats kv add url-expand-cache --replicas 3 --storage file
```

`EnsureKeyValue` loads an existing bucket as-is, so the pre-created config is kept.
