# 200-url-shortener-maria

URL shortener with MariaDB direct (no cache, no pooler). Prefork on with pool `max_conns=100` per child (50 idle). Uses SDK `type: crud` — no Fiber import in user code.

**Stack:** Fiber (prefork) + MariaDB 11 (via `database/sql` + `go-sql-driver/mysql`, each child gets its own pool).

## Configuration

| Variable | Value | Description |
|----------|-------|-------------|
| `DATABASE_URL` | `dev:devpass@tcp(mariadb:3306)/shorturl` | MariaDB direct connection |
| `CONFIG_PATH` | `service.docker.yaml` | Prefork on, pool max_conns=100 |

Pool config is set via YAML (`pool.max_conns` / `min_conns`) and applied by `initMySQL` in the SDK (`SetMaxOpenConns` / `SetMaxIdleConns`).

## Quick Start

```bash
docker compose up --build -d
docker compose logs bench -f
docker compose down -v
```

## Endpoints

Same CRUD set as all other examples: `POST/GET/GET/PUT/DELETE /links` + `GET /expand/:shortCode`. Uses `type: crud` — auto-generated routes.

## Benchmark (wrk -t10 -c1000 -d30s, 1 measure run)

| Endpoint | RPS | ±5% | ±10% |
|----------|:---:|:---:|:----:|
| Expand (GET /expand/:shortCode) | 53,984 | 51,285–56,683 | 48,586–59,382 |
| List (GET /links) | 24,434 | 23,212–25,656 | 21,991–26,877 |
| GetByID (GET /links/:id) | 44,157 | 41,949–46,365 | 39,741–48,573 |
| Create (POST /links) | 10,566 | 10,038–11,094 | 9,509–11,623 |
| Update (PUT /links/:id) | 184,178 | 174,969–193,387 | 165,760–202,596 |
| Delete (DELETE /links/:id) | 36,486 | 34,662–38,310 | 32,837–40,135 |

**Performance notes:** Pool `max_conns=20` (PgDog-like), `innodb_flush_log_at_trx_commit=2`, `innodb_buffer_pool_size=256M`. Without these tunings, MariaDB delete benchmarked at 442 RPS (InnoDB fsync per transaction bottleneck).

## Architecture

| File | Purpose |
|------|---------|
| `models/link.go` | Link model (primary key: `id`) |
| `models/link_expand.go` | LinkExpand model (primary key: `short_code`) |
| `hooks.go` | `BeforeCreate` auto-generates short codes |
| `main.go` | `MySQLMustRegister` (no cache) |
| `service.docker.yaml` | Prefork on, pool=20 (PgDog-like), CRUD entries |
| `bench_test.go` | Functional tests + BenchmarkExpand |
| `run.sh` | Entrypoint: functional tests always, RPS benchmark only with `RPS_BENCH=1` (6 endpoints) |
| `docker-compose.yml` | MariaDB 11 (max_connections=2000) |
