# 100-healthz — Minimal HTTP throughput

Simple HTTP health-check endpoint with zero dependencies. Measures raw Fiber vs SDK middleware overhead.

**Stack:** Fiber v3 (fasthttp) + SDK middleware.

## Quick Start

```bash
# Single entry point — builds, tests, benches, exits
docker compose up --abort-on-container-exit
```

## Expected RPS (wrk -t10 -c1000 -d15s)

| Mode | RPS |
|------|-----|
| Raw Fiber | ~743k |
| SDK middleware | ~727k |

## Architecture

```
docker compose up → build image → bench container starts
  ↓
run.sh (Docker CMD):
  1. /app/tester -test.run=TestHealthz_OK   ← functional test (200 OK)
  2. /app/svc & → wrk raw Fiber → kill
  3. /app/svc & → wrk SDK middleware → kill
  ↓
container exit 0 → compose stops
```

| File | Purpose |
|------|---------|
| `bench_test.go` | TestHealthz_OK + Go benchmarks (compiled into /app/tester) |
| `run.sh` | Docker CMD: functional test → wrk benchmarks |
| `Dockerfile` | Multi-stage: builds svc + tester binaries |
| `docker-compose.yml` | Single bench container, no DB dependencies |
