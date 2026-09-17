# 200-url-shortener-mariadb

URL shortener with MariaDB via PgDog proxy.

## Quick Start

```bash
docker compose run --rm bench               # functional tests
docker compose run --rm bench --rps         # functional + RPS
```

## Benchmark (wrk -t10 -c1000 inside Docker via ProxySQL)

| Endpoint | Dedicated (12-core) | Local (10-core) | Baseline |
|----------|:---:|:---:|:---:|
| Expand (GET /expand/:shortCode) | 38,981 | 29,051 | 34,014 |
| List (GET /links) | 28,132 | 19,968 | 23,336 |
| GetByID (GET /links/:id) | 38,824 | 31,677 | 37,857 |
| Create (POST /links) | 38,229 | 23,698 | 16,658 |
| Update (PATCH /links/:id) | **39,250** | 28,733 | 34,382 |
| Delete (DELETE /links/:id) | **38,425** | 26,918 | 35,066 |

Update and Delete measured isolated on a clean dedicated box with warm MariaDB.
The previous dedicated values (8,730 / 9,596) were anomalous — row-lock
contention in a full run with hot id ranges under 1000 connections. The baseline
update (34K) already uses the PATCH fix.

Measured 2026-08-20 on v0.18.2 (Dedicated = 12-core AMD Linux; wrk inside
Docker, clean host). Baseline 2026-08-01.

> **2026-09 anomaly (fixed):** the 2026-09-01 VPS sweep measured expand
> 106k→24k with `Error 1226 (max_user_connections)` — ProxySQL capped the
> `dev` user and the hostgroup at 200 connections while prefork × pool opens
> up to ~1200. Both caps raised to 1500 in `proxysql.cnf` (recreate the
> proxysql container after editing: it only reads the file on a fresh
> datadir). Re-measured clean on Docker/Mac: expand ~32k warmup/measure with
> zero non-2xx. Also fixed: PATCH with JSON keys (`targetUrl`) returned 500
> (`no such column`) — the SDK now maps JSON names to columns on update.

## Architecture

| File | Purpose |
|------|---------|
| `cmd/main.go` | Bootstrap — MySQLMustRegister × 2 |
| `models/link.go` | Link model + BeforeCreate hook (auto short code) |
| `models/link_expand.go` | LinkExpand model (read-only, PK: short_code) |
| `service.yaml` | Service config (api_prefix: /api) |
| `service.docker.yaml` | Docker config override |
| `run.sh` | Entrypoint: --rps for benchmarks, --test:Name for specific tests |
| `bench_test.go` | Functional tests + expand benchmark |
| `docker-compose.yml` | Services: mariadb + proxysql (pooler) + bench |
| `proxysql.cnf` | ProxySQL configuration for connection pooling |
