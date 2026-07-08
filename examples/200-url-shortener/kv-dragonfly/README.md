# 200-url-shortener-kv

URL shortener using Dragonfly as a primary key-value store (no SQL database, no MongoDB). No CRUD driver — all 6 handlers are manual REST via `svc.WithRest` + `*RestCtx`. The YAML has no `databases:` block. Minimal middleware (`apply: []`). Prefork on. No Fiber import in user code.

**Stack:** Dragonfly KV + SDK `RestCtx` (no Fiber import in user code).

## Configuration

| Variable | Value | Description |
|----------|-------|-------------|
| `DRAGONFLY_ADDR` | `dragonfly:6379` | Dragonfly connection |

## Endpoints

Same CRUD set as all other examples: `POST/GET/GET/PUT/DELETE /links` + `GET /expand/:shortCode`.

## Benchmark (wrk -t10 -c1000 -d30s, 3 runs each endpoint)

All 6 endpoints are benchmarked sequentially within the same container (200 pre-seeded keys). DELETE exhausts keys quickly (~10ms) — the RPS measures framework throughput including 404s.

| Endpoint | Run 1 | Run 2 | Run 3 | Average |
|----------|:-----:|:-----:|:-----:|:-----:|
| Expand (GET /expand/:shortCode) | 120,682 | 132,805 | 126,878 | **126,788** |
| List (GET /links) | 1,279 | 1,336 | 1,177 | **1,264** |
| GetByID (GET /links/:id) | 126,844 | 122,055 | 126,060 | **124,986** |
| Create (POST /links) | 64,932 | 56,419 | 66,917 | **62,756** |
| Update (PUT /links/:id) | 65,555 | 56,818 | 68,232 | **63,535** |
| Delete (DELETE /links/:id) | 66,136 | 64,465 | 73,417 | **68,006** |

## Architecture

| File | Purpose |
|------|---------|
| `main.go` | 6 REST handlers (create/list/get/update/delete/expand) via `svc.WithRest` + `*RestCtx` — no Fiber import |
| `service.docker.yaml` | 6 `type: rest` entries, `apply: []` middleware, no `databases:` |
| `bench_test.go` | Functional tests + BenchmarkExpand |
| `run.sh` | Entrypoint: functional tests always, RPS benchmark only with `RPS_BENCH=1` (6 endpoints) |
| `docker-compose.yml` | Dragonfly + bench container |

KV data model: `link:next_id` (counter), `link:id:<id>` (JSON), `link:sc:<shortCode>` (JSON) — reverse lookup for expand.
