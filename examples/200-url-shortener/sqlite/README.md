# 200-url-shortener-turso

URL shortener with Turso (embedded libSQL via SDK `runtime.TursoMustRegister`). No external database server — the database is a local file. Uses `_busy_timeout=30000` (built-in Turso driver param) and pool `max_conns=500` for write queuing. Prefork disabled. Uses SDK `type: crud` — no Fiber import in user code, no direct tursogo dependency.

**Stack:** Fiber + Turso libSQL via SDK (busy_timeout built-in, WAL mode).

## Configuration

| Variable | Value | Description |
|----------|-------|-------------|
| `DATABASE_URL` | `/app/data/shorturl.db?_busy_timeout=30000` | Turso with built-in busy timeout |
| `CONFIG_PATH` | `service.docker.yaml` | No prefork, pool=500 |
| `MaxOpenConns` | 500 (via pool config) | Concurrent reads |
| `busy_timeout` | 30s (DSN param) | SQLite waits up to 30s for the write lock |
| `turso.mode` | `local` (YAML) | Embedded local mode |

YAML:
```yaml
databases:
  - driver: turso
    url: "${DATABASE_URL}"
    pool:
      max_conns: 500
    turso:
      mode: local
      busy_timeout: 30000
```

## Quick Start

```bash
docker compose run --rm bench               # functional tests
docker compose run --rm bench --rps         # functional + RPS
```

## Benchmark (wrk -t10 -c1000 inside Docker)

| Endpoint | Dedicated (12-core) | Local (10-core) | Baseline |
|----------|:---:|:---:|:---:|
| Expand (GET /expand/:shortCode) | 64,561 | 56,829 | 57,882 |
| List (GET /links) | 31,876 | 25,845 | 23,672 |
| GetByID (GET /links/:id) | 63,449 | 53,769 | 54,924 |
| Create (POST /links) | 40.88 | 14.08 | 19.06 |
| Update (PUT /links/:id) | 78.34 | 36.89 | 7,254* |
| Delete (DELETE /links/:id) | 30.03 | 6.30 | 7.06 |

\* The baseline Update (7,254) was invalid: `update.lua` used `PUT` against a
PATCH-only route, returning 405 without touching the database (see
docs/benchmarks.md rules 10-11). Real SQLite single-writer writes are tens of
RPS.

Measured 2026-08-19 on v0.18.2 (Dedicated = 12-core AMD Linux box; Local =
10-core ARM macOS; wrk inside Docker, clean host before measuring). Baseline
2026-08-01.

## Limitations

- **libSQL single-writer:** Create/Delete/Update serialize on a single write slot. `busy_timeout=30000` makes SQLite wait up to 30s for the lock. MVCC + `BEGIN CONCURRENT` does not work with the Go driver.
- **Prefork off:** multi-process WAL degrada writes severamente.
- **Reads vs Writes:** Reads ~55k RPS. Writes ~7-19 RPS — ~3,000-7,000× menos.

## Architecture

| File | Purpose |
|------|---------|
| `cmd/main.go` | Bootstrap — `TursoMustRegister` × 2 |
| `models/link.go` | Link model + `BeforeCreate` hook |
| `models/link_expand.go` | LinkExpand model (PK: short_code) |
| `service.yaml` | Service config (api_prefix: /api) |
| `service.docker.yaml` | Docker config (no prefork, pool=500, local mode) |
| `bench_test.go` | Functional tests + BenchmarkExpand |
| `run.sh` | Entrypoint: `--rps` for benchmarks, `--test:Name` for specific tests |
| `docker-compose.yml` | Bench container only (DB embedded) |
| `turso-init.go` | Build tag init for Turso C library cache |
