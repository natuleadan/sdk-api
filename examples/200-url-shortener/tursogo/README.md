# 200-url-shortener - tursogo (local embedded)

URL shortener using the **tursogo** driver (`driver: turso`) - embedded Turso
engine on a local SQLite-compatible file. No external database server, no
credentials. This is the "sqlite without a server" variant: the embedded
engine IS SQLite under the hood.

**Stack:** SDK `type: crud` (no Fiber import) + tursogo local.

## Run

```bash
DATABASE_URL=/tmp/shortener.db CONFIG_PATH=service.yaml go run ./cmd
# or inside Docker (seeds 200 hot keys + functional tests):
docker compose up --build
```

## API

- `POST /api/links` `{"targetUrl":"https://example.com"}` -> creates a short code
- `GET /api/links` -> list
- `GET /api/links/:id` -> get by id
- `PATCH /api/links/:id` -> update
- `DELETE /api/links/:id` -> delete
- `GET /api/expand/:shortCode` -> resolves to target URL

## Benchmarks (wrk -t10 -c1000 inside Docker, local file DB)

| Endpoint | req/s |
|----------|:-----:|
| Expand (GET /expand/:shortCode) | 54,371 |
| GetByID (GET /links/:id) | 49,815 |
| List (GET /links) | 24,101 |
| Delete (DELETE /links/:id) | 16,326 |
| Update (PATCH /links/:id) | 2,966 |
| Create (POST /links) | 289 |

> Local embedded engine: no network, so reads are fast; writes are
> serialized by the single-file SQLite write lock.

## Notes

- Uses `turso.mode: local` + `busy_timeout` (YAML), pool for concurrent writes.
- No env vars besides `DATABASE_URL` (local path).
