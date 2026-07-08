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

## Benchmark (wrk -t10 -c1000 -d30s, 3 runs each endpoint)

| Endpoint | Run 1 | Run 2 | Run 3 | Average |
|----------|:-----:|:-----:|:-----:|:-------:|
| Expand (GET /expand/:shortCode) | 27,336 | 27,145 | 27,510 | **27,330** |
| List (GET /links) | 4,321 | 4,469 | 4,759 | **4,516** |
| GetByID (GET /links/:id) | 28,792 | 29,416 | 27,201 | **28,469** |
| Create (POST /links) | 25,414 | 29,296 | 27,543 | **27,417** |
| Update (PUT /links/:id) | 99,767 | 97,338 | 92,725 | **96,610** |
| Delete (DELETE /links/:id) | 29,748 | 30,480 | 29,646 | **29,958** |

## Architecture

| File | Purpose |
|------|---------|
| `main.go` | `MongoMustRegister` with two models |
| `service.docker.yaml` | Prefork on, pool=150, CRUD entries |
| `bench_test.go` | Functional tests + BenchmarkExpand |
| `run.sh` | Entrypoint: functional tests always, RPS benchmark only with `RPS_BENCH=1` (6 endpoints) |
| `docker-compose.yml` | MongoDB 7 + bench container |
