# 202-url-link-nats — URL shortener with NATS KV cache

CRUD-based URL shortener. Creates short codes → expands via NATS KV cache with PostgreSQL fallback.

**Stack:** Fiber + pgx + NATS JetStream + PgDog pooler.

## Quick Start

```bash
docker compose up --abort-on-container-exit
```

## Expected RPS (wrk -t10 -c1000 -d15s)

| Mode | RPS |
|------|-----|
| Expand (NATS KV cache hit) | ~133k-141k |

## Architecture

```
docker compose up → PG + NATS + PgDog start → bench container starts
  ↓
run.sh (Docker CMD):
  1. /app/svc & → health check
  2. /app/tester -test.run=TestURLLink   ← functional tests
  3. seed 100 hot keys via curl
  4. wrk expand benchmark
  ↓
container exit 0 → compose stops
```

| File | Purpose |
|------|---------|
| `bench_test.go` | TestURLLink_CreateAndExpand + TestURLLink_CRUDLifecycle + BenchmarkExpand |
| `run.sh` | Docker CMD: start svc → tester → seed → wrk |
| `Dockerfile` | Multi-stage: builds svc + tester binaries |
| `docker-compose.yml` | PG + NATS + PgDog + bench |
| `pgdog.toml` | PgDog transaction pooler config |
| `users.toml` | PgDog user mapping (matches PG roles) |
