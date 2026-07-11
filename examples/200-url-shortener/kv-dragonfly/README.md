# 200-url-shortener-kv

URL shortener using Dragonfly as a primary key-value store (no SQL database, no MongoDB). No CRUD driver — all 6 handlers are manual REST via `svc.WithRest` + `*RestCtx`. The YAML has no `databases:` block. Minimal middleware (`apply: []`). Prefork on. No Fiber import in user code.

**Stack:** Dragonfly KV + SDK `RestCtx` (no Fiber import in user code).

## Configuration

| Variable | Value | Description |
|----------|-------|-------------|
| `DRAGONFLY_ADDR` | `dragonfly:6379` | Dragonfly connection |

## Endpoints

Same CRUD set as all other examples: `POST/GET/GET/PUT/DELETE /links` + `GET /expand/:shortCode`.

## Benchmark (wrk -t10 -c1000 -d30s)

| Endpoint | RPS | ±5% | ±10% |
|----------|:---:|:---:|:----:|
| Expand (GET /expand/:shortCode) | 126,788 | 120,449–133,127 | 114,109–139,467 |
| List (GET /links) | 1,264 | 1,201–1,327 | 1,138–1,390 |
| GetByID (GET /links/:id) | 124,986 | 118,737–131,235 | 112,487–137,485 |
| Create (POST /links) | 62,756 | 59,618–65,894 | 56,480–69,032 |
| Update (PUT /links/:id) | 63,535 | 60,358–66,712 | 57,182–69,889 |
| Delete (DELETE /links/:id) | 68,006 | 64,606–71,406 | 61,205–74,807 |

## Architecture

| File | Purpose |
|------|---------|
| `main.go` | 6 REST handlers (create/list/get/update/delete/expand) via `svc.WithRest` + `*RestCtx` — no Fiber import |
| `service.docker.yaml` | 6 `type: rest` entries, `apply: []` middleware, no `databases:` |
| `bench_test.go` | Functional tests + BenchmarkExpand |
| `run.sh` | Entrypoint: functional tests always, RPS benchmark only with `RPS_BENCH=1` (6 endpoints) |
| `docker-compose.yml` | Dragonfly + bench container |

KV data model: `link:next_id` (counter), `link:id:<id>` (JSON), `link:sc:<shortCode>` (JSON) — reverse lookup for expand.
