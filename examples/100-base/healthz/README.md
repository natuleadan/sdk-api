# 100-healthz — Minimal HTTP throughput

Simple HTTP health-check endpoint with zero dependencies. Measures raw Fiber vs SDK middleware overhead.

**Stack:** Fiber v3 (fasthttp) + SDK middleware.

## Quick Start

```bash
# From root runner
cd examples && ./run.sh 100            # functional tests
cd examples && ./run.sh 100 --rps      # functional + RPS (wrk inside Docker)

# Or directly
cd examples/100-healthz
docker compose run --rm bench               # functional tests
docker compose run --rm bench --rps         # functional + RPS
```

## Benchmark (wrk -t10 -c1000 inside Docker)

| Endpoint | Dedicated (12-core) | Local (10-core) | Baseline |
|----------|:---:|:---:|:---:|
| Healthz (GET /healthz) | 545,449 | 661,742 | 637,818 |

Measured 2026-08-19 on v0.18.2. wrk runs inside the same container as the
service (no host networking overhead). Dedicated = 12-core AMD Linux box;
Local = 10-core ARM macOS. The baseline (637,818) was measured 2026-08-02 on
the local machine.

## Architecture

```
docker compose run bench → build image → container starts
  ↓
run.sh:
  1. /app/svc & → wait /healthz
  2. /app/tester -test.run=TestHealthz_OK
  3. [--rps] wrk -t10 -c1000 -d3s warmup (discarded) + -d5s measure
  ↓
container exit → compose cleans
```

| File | Purpose |
|------|---------|
| `run.sh` | Entrypoint: `--rps` for benchmarks, `--test:Name` for specific tests |
| `Dockerfile` | Multi-stage: builds svc + tester, installs wrk |
| `docker-compose.yml` | Single bench container, no DB dependencies |
