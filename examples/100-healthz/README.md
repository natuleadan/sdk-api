# 100-healthz — Minimal HTTP throughput

Simple HTTP health-check endpoint with zero dependencies. Measures raw Fiber vs SDK middleware overhead.

**Stack:** Fiber v3 (fasthttp) + SDK middleware.

## Quick Start

```bash
# Single entry point — builds, tests, benches, exits
docker compose up --abort-on-container-exit
```

## Benchmark (wrk -t10 -c1000 -d15s, 3 runs)

| Mode | Run 1 | Run 2 | Run 3 | Average |
|------|:-----:|:-----:|:-----:|:-----:|
| Raw Fiber | 729,998 | 742,123 | 745,646 | **739,256** |
| SDK middleware | 686,174 | 710,107 | 749,452 | **715,244** |

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
