# 201-url-link-base — URL shortener with Redis cache

CRUD-based URL shortener. Creates short codes → expands via Redis cache with PostgreSQL fallback.

**Stack:** Fiber + pgx + Redis + PgDog pooler.

## Quick Start

```bash
docker compose up --abort-on-container-exit
```

## Expected RPS (wrk -t10 -c1000 -d15s)

| Mode | RPS |
|------|-----|
| Expand (Redis cache hit) | ~124k-151k |

## Architecture

```
docker compose up → PG + Redis + PgDog start → bench container starts
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
| `docker-compose.yml` | PG + Redis + PgDog + bench |
| `pgdog.toml` | PgDog transaction pooler config |
| `users.toml` | PgDog user mapping (matches PG roles) |
