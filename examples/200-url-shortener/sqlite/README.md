# 200-url-shortener-turso

URL shortener with Turso (embedded libSQL via `tursogo v0.6.1`). No external database server — the database is a local file. Uses `_busy_timeout=30000` (built-in Turso driver param) and pool `max_conns=500` for write queuing. Prefork disabled. Uses SDK `type: crud` — no Fiber import in user code.

**Stack:** Fiber + Turso libSQL (v0.6.1 purego, busy_timeout built-in, WAL mode).

## Configuration

| Variable | Value | Description |
|----------|-------|-------------|
| `DATABASE_URL` | `/app/data/shorturl.db?_busy_timeout=30000` | Turso with built-in busy timeout |
| `CONFIG_PATH` | `service.docker.yaml` | No prefork, pool=500 |
| `MaxOpenConns` | 500 (via pool config) | Concurrent reads |
| `busy_timeout` | 30s (DSN param) | SQLite espera hasta 30s por el lock |
| `turso.mode` | `local` (YAML) | Local embebido |

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
docker compose up --abort-on-container-exit
```

## Benchmark (wrk -t10 -c1000 -d30s)

| Endpoint | RPS | ±5% | ±10% |
|----------|:---:|:---:|:----:|
| Expand (GET /expand/:shortCode) | 60,768 | 57,730–63,806 | 54,691–66,845 |
| List (GET /links) | 5,127 | 4,871–5,383 | 4,614–5,640 |
| GetByID (GET /links/:id) | 59,378 | 56,409–62,347 | 53,440–65,316 |
| Create (POST /links) | 12.91 | 12.26–13.56 | 11.62–14.20 |
| Update (PUT /links/:id) | 81,974 | 77,875–86,073 | 73,777–90,171 |
| Delete (DELETE /links/:id) | 21,471 | 20,397–22,545 | 19,324–23,618 |

## Limitations

- **libSQL single-writer:** Create/Delete serialize on the single writer slot. `busy_timeout=30000` makes SQLite wait up to 30s for the lock instead of failing immediately. MVCC + `BEGIN CONCURRENT` does not work with the Go driver (`database/sql` wrapper is incompatible with concurrent transactions).
- **Prefork off:** multi-process WAL degrades writes severely (Create dropped from 16 to 0.7 RPS). Without MPW, v0.6.1 with `_busy_timeout` DSN gives the best results.
- **Reads vs Writes:** Reads achieve ~60k RPS. Writes (Create) achieve ~13 RPS due to the single-writer bottleneck — ~4,600× less than reads.
- **v0.6.1 vs v0.7.0-pre.17:** v0.6.1 (stable) with purego (no CGO) gives better RPS on both reads and writes than v0.7.0-pre.17 with CGO.

## Architecture

| File | Purpose |
|------|---------|
| `models/link.go` | Link model (primary key: `id`) |
| `models/link_expand.go` | LinkExpand model (primary key: `short_code`) |
| `hooks.go` | `BeforeCreate` auto-generates short codes |
| `main.go` | `TursoMustRegister` |
| `service.docker.yaml` | Docker config (no prefork, pool=500, local mode) |
| `bench_test.go` | Functional tests + BenchmarkExpand |
| `run.sh` | Entrypoint: functional tests always, RPS benchmark only with `RPS_BENCH=1` (6 endpoints) |
| `docker-compose.yml` | Bench container only (DB embedded) |
