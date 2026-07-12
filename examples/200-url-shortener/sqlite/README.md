# 200-url-shortener-turso

URL shortener with Turso (embedded libSQL via `tursogo v0.6.1`). No external database server — the database is a local file. Uses `_busy_timeout=30000` (built-in Turso driver param) and pool `max_conns=500` for write queuing. Prefork disabled. Uses SDK `type: crud` — no Fiber import in user code.

**Stack:** Fiber + Turso libSQL (v0.6.1 purego, busy_timeout built-in, WAL mode).

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
| Expand (GET /expand/:shortCode) | 75,775 | Direct SQLite read |
| List (GET /links) | 5,527 | Offset pagination with COUNT(*) |
| GetByID (GET /links/:id) | 72,703 | Direct read by PK |
| Create (POST /links) | 11.84 | Single-writer SQLite |
| Update (PUT /links/:id) | 6,673 | Write + busy_timeout |
| Delete (DELETE /links/:id) | 12.40 | Single-writer SQLite |

## Limitations

- **libSQL single-writer:** Create/Delete/Update serialize on a single write slot. `busy_timeout=30000` makes SQLite wait up to 30s for the lock. MVCC + `BEGIN CONCURRENT` does not work with the Go driver.
- **Prefork off:** multi-process WAL degrada writes severamente.
- **Reads vs Writes:** Reads ~70k RPS. Writes ~10 RPS — ~7,000× menos.
- **Update 81,974 from previous README was incorrect** — a PUT in SQLite cannot exceed ~10 RPS due to single-writer. That value was copied from another endpoint by mistake.

## Architecture

| File | Purpose |
|------|---------|
| `models/link.go` | Link model (primary key: `id`) |
| `models/link_expand.go` | LinkExpand model (primary key: `short_code`) |
| `hooks.go` | `BeforeCreate` auto-generates short codes |
| `main.go` | `TursoMustRegister` |
| `service.docker.yaml` | Docker config (no prefork, pool=500, local mode) |
| `bench_test.go` | Functional tests + BenchmarkExpand |
| `run.sh` | Entrypoint: `--rps` for benchmarks, `--test:Name` for specific tests |
| `docker-compose.yml` | Bench container only (DB embedded) |
