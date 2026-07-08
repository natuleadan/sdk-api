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
docker compose up --abort-on-container-exit
```

## Endpoints

Same CRUD set as all other examples: `POST/GET/GET/PUT/DELETE /links` + `GET /expand/:shortCode`. Uses `type: crud` — auto-generated routes.

## Benchmark (wrk -t10 -c1000 -d30s, 2 runs)

| Endpoint | Run 1 (cold) | Run 2 | Run 3 | Average (warm) |
|----------|:-----------:|:-----:|:-----:|:--------------:|
| Expand (GET /expand/:shortCode) | 3,292 | 38,813 | 42,851 | **40,832** |
| List (GET /links) | 5,700 | 25,555 | 26,167 | **25,861** |
| GetByID (GET /links/:id) | 7,355 | 40,649 | 40,675 | **40,662** |
| Create (POST /links) | 4,109 | 3,307 | 2,969 | **3,138** |
| Update (PUT /links/:id) | 102,809 | 105,697 | 102,476 | **104,086** |
| Delete (DELETE /links/:id) | 203 | 316 | 568 | **442** |

Run 1 suffered from cold connection pool + `max_connections` saturation (`invalid connection` errors). Runs 2-3 use `min_conns=50` + `--max_connections=2000`.

## Architecture

| File | Purpose |
|------|---------|
| `models/link.go` | Link model (primary key: `id`) |
| `models/link_expand.go` | LinkExpand model (primary key: `short_code`) |
| `hooks.go` | `BeforeCreate` auto-generates short codes |
| `main.go` | `MySQLMustRegister` (no cache) |
| `service.docker.yaml` | Prefork on, pool=100, CRUD entries |
| `bench_test.go` | Functional tests + BenchmarkExpand |
| `run.sh` | Entrypoint: functional tests always, RPS benchmark only with `RPS_BENCH=1` (6 endpoints) |
| `docker-compose.yml` | MariaDB 11 (max_connections=2000) |
