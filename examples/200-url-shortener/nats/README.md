# url-shortener-nats

URL shortener CRUD with **NATS JetStream events**, **core NATS Request-Reply**, and **NATS KeyValue** store.

## Architecture

- **PostgreSQL** — primary data store (via pgdog connection pooler)
- **NATS JetStream** — event stream for CRUD operations (created/updated/deleted)
- **Core NATS** — request-reply RPC
- **NATS KV** — embedded key-value store

## Endpoints

| Endpoint | Type | NATS |
|----------|------|------|
| `POST /links` | CRUD Create | Publishes `links.created` event |
| `GET /links` | CRUD List | — |
| `GET /links/:id` | CRUD Get | — |
| `PATCH /links/:id` | CRUD Update | Publishes `links.updated` event |
| `DELETE /links/:id` | CRUD Delete | Publishes `links.deleted` event |
| `GET /expand/:shortCode` | REST Expand | Publishes `links.expanded` event |
| `POST /nats/rpc` | Request-Reply | Core NATS request (echo handler) |
| `GET /nats/kv/:key` | KV Get | NATS KeyValue read |
| `PUT /nats/kv/:key` | KV Set | NATS KeyValue write |
| `POST /nats/pull` | Pull Publish | JetStream pull consumer |
| `GET /admin/events` | Admin | In-memory event log |

## RPS Benchmark

**Environment:** Apple Silicon (10c/3GHz ARM), Docker Compose, single process (prefork disabled).

| Endpoint | RPS | ±5% | ±10% |
|----------|:---:|:---:|:----:|
| list | 21,782 | 20,693–22,871 | 19,604–23,960 |
| expand | 94,829 | 90,088–99,570 | 85,346–104,312 |
| create | 16,821 | 15,980–17,662 | 15,139–18,503 |
| update | 14,417 | 13,696–15,138 | 12,975–15,859 |
| delete | 37,441 | 35,569–39,313 | 33,697–41,185 |
| rpc | 100,007 | 95,007–105,007 | 90,006–110,008 |
| kv-get | 93,192 | 88,532–97,852 | 83,873–102,511 |
| kv-set | 87,791 | 83,401–92,181 | 79,012–96,570 |

- **list/expand/create/update/delete** — include PostgreSQL writes/reads + NATS JetStream event publish via `PublishJSON`
- **rpc** — pure core NATS request-reply (no PG, no JetStream)
- **kv-get/kv-set** — pure NATS KeyValue operations (no PG, no JetStream)
- Prefork disabled (required for NATS — each process owns its own connections and exit workers)

## Run

```bash
# Functional tests only
docker compose up --build -d
docker compose logs bench -f
docker compose down -v

# Functional tests + RPS benchmarks
RPS_BENCH=1 docker compose up --build -d
docker compose logs bench -f
# wait for "Benchmark complete", then:
docker compose down -v
```
