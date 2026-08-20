# 200-url-shortener-postgres-mem-dragonfly

URL shortener with PostgreSQL + RAM L1 + Dragonfly L2.

## Quick Start

```bash
docker compose run --rm bench               # functional tests
docker compose run --rm bench --rps         # functional + RPS
```

## Benchmark (wrk -t10 -c1000 inside Docker)

| Endpoint | Dedicated (12-core) | Local (10-core) | Baseline |
|----------|:---:|:---:|:---:|
| Expand (GET /api/expand/:shortCode) | **304,878** | 258,684 | 259,606 |
| Create (POST /api/links) | **299,160** | 227,489 | 17,633 |
| Delete (DELETE /api/links/:id) | **52,957** | 30,137 | 34,239 |
| GetByID (GET /api/links/:id) | **64,954** | 40,801 | 42,025 |
| List (GET /api/links) | **30,651** | 22,099 | 20,790 |
| Update (PATCH /api/links/:id) | **26,687** | 23,907 | 16,854 |

Measured 2026-08-19 on v0.18.2 (Dedicated = 12-core AMD Linux box; Local =
10-core ARM macOS; same code, same tag, wrk inside Docker).

### Methodology

- **Expand** is measured isolated (`--rps:expand`) with a warm L1 cache. In the
  full `--rps` run the expand drops to ~30K because create/update/delete
  **invalidate** the L1 entries that expand reads. The real warm-cache value is
  258-305K.
- **Create 299K vs baseline 17.6K**: the baseline (2026-08-01) was measured
  under host memory/swap pressure. On a clean dedicated box the local PG INSERT
  yields ~17x more.
- Dedicated wins every endpoint (+12% to +76%) — more cores and RAM, no host
  contention. The baseline expand (259,606) matches the local 258,684, so the
  methodology is consistent.

## Architecture

| File | Purpose |
|------|---------|
| `cmd/main.go` | Bootstrap — MustRegister + CachedCRUD with L1+L2 |
| `models/link.go` | Link model + BeforeCreate hook |
| `models/link_expand.go` | LinkExpand model (PK: short_code, L1+L2 cached) |
| `service.yaml` | Service config (api_prefix: /api) |
| `service.docker.yaml` | Docker config (prefork, pool, PgDog) |
| `run.sh` | Entrypoint: --rps for benchmarks, --test:Name for specific tests |
| `bench_test.go` | Functional tests + expand benchmark |
| `docker-compose.yml` | PostgreSQL 18 + PgDog + Dragonfly |
