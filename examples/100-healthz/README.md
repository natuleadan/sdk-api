# 100-healthz — Minimal HTTP throughput

Simple HTTP health-check endpoint with zero dependencies. Measures raw Fiber vs SDK middleware overhead.

**Stack:** Fiber v3 (fasthttp) + SDK middleware.

## Quick Start

```bash
# Single entry point — builds, tests, benches, exits
docker compose up --abort-on-container-exit
```

## Benchmark (wrk -t10 -c1000 -d15s)

| Mode | RPS | ±5% | ±10% |
|------|:---:|:---:|:----:|
| Raw Fiber | 739,256 | 702,293–776,219 | 665,330–813,182 |
| SDK middleware | 715,244 | 679,482–751,006 | 643,720–786,768 |

## Architecture

```
docker compose up → build image → bench container starts
  ↓
run-test-logic.sh (Docker CMD via run.sh):
  1. /app/svc & → wait /healthz
  2. /app/tester -test.run=TestHealthz_OK   ← functional test (200 OK)
  ↓
container exit 0 → compose stops
```

| File | Purpose |
|------|---------|
| `bench_test.go` | `TestHealthz_OK` + Go benchmarks (compiled into /app/tester) |
| `run.sh` | Entrypoint: functional tests only (`RPS_BENCH=1` has no effect — no wrk in healthz) |
| `Dockerfile` | Multi-stage: builds svc + tester binaries |
| `docker-compose.yml` | Single bench container, no DB dependencies |
