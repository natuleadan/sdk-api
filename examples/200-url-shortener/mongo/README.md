# 200-url-shortener-mongo

URL shortener with MongoDB as the database backend. Prefork on with `maxPoolSize=150` per child (`max_conns=150` via YAML). Uses SDK `type: crud` — no Fiber import in user code.

**Stack:** Fiber (prefork) + MongoDB 7 (via `infra/stores/mon`, each child gets its own client).

## Configuration

| Variable | Value | Description |
|----------|-------|-------------|
| `MONGO_URI` | `mongodb://mongo:27017` | MongoDB connection string |
| `CONFIG_PATH` | `service.docker.yaml` | Prefork on, pool max_conns=150 |

MongoDB `maxPoolSize` is set via the `pool.max_conns` YAML field (appends `?maxPoolSize=N&maxConnecting=10` to URI). Each prefork child creates its own singleton client with its own connection pool — matching the recommended architecture.

## Quick Start

```bash
docker compose up --abort-on-container-exit
```

## Endpoints

Same CRUD set as all other examples: `POST/GET/GET/PUT/DELETE /links` + `GET /expand/:shortCode`. Uses `type: crud` — auto-generated routes.

## Benchmark (wrk -t10 -c1000 -d30s)

| Endpoint | RPS | ±5% | ±10% |
|----------|:---:|:---:|:----:|
| Expand (GET /expand/:shortCode) | 27,330 | 25,964–28,697 | 24,597–30,063 |
| List (GET /links) | 4,516 | 4,290–4,742 | 4,064–4,968 |
| GetByID (GET /links/:id) | 28,469 | 27,046–29,892 | 25,622–31,316 |
| Create (POST /links) | 27,417 | 26,046–28,788 | 24,675–30,159 |
| Update (PUT /links/:id) | 96,610 | 91,780–101,441 | 86,949–106,271 |
| Delete (DELETE /links/:id) | 29,958 | 28,460–31,456 | 26,962–32,954 |

## Architecture

| File | Purpose |
|------|---------|
| `main.go` | `MongoMustRegister` with two models |
| `service.docker.yaml` | Prefork on, pool=150, CRUD entries |
| `bench_test.go` | Functional tests + BenchmarkExpand |
| `run.sh` | Entrypoint: functional tests always, RPS benchmark only with `RPS_BENCH=1` (6 endpoints) |
| `docker-compose.yml` | MongoDB 7 + bench container |
