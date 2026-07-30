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

| Endpoint | RPS | Notes |
|----------|:---:|-------|
| Expand (GET /expand/:shortCode) | 57,882 | Direct SQLite read |
| List (GET /links) | 23,672 | Offset pagination with COUNT(*) |
| GetByID (GET /links/:id) | 54,924 | Direct read by PK |
| Create (POST /links) | 19.06 | Single-writer SQLite |
| Update (PUT /links/:id) | 7,254 | Write + busy_timeout |
| Delete (DELETE /links/:id) | 7.06 | Single-writer SQLite |

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
