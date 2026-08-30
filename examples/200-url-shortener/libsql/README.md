# 200-url-shortener - libsql (remote hrana)

URL shortener using the **libsql-client-go** driver (`driver: libsql`) -
remote Turso Cloud / Bunny over the **hrana wire protocol**. Pure Go.

Same code as the other variants (same `cmd/`, `models/`, CRUD); only the
database driver and connection env vars differ.

## Run

```bash
export TURSO_DATABASE_URL="libsql://<db>.turso.io"   # or Bunny
export TURSO_AUTH_TOKEN="<token>"
go run ./cmd
# or inside Docker (seeds 200 hot keys + functional tests + RPS):
docker compose up --build
```

## API

- `POST /api/links` `{"targetUrl":"https://example.com"}` -> creates a short code
- `GET /api/links` -> list
- `GET /api/links/:id` -> get by id
- `PATCH /api/links/:id` -> update
- `DELETE /api/links/:id` -> delete
- `GET /api/expand/:shortCode` -> resolves to target URL

## Benchmarks (wrk -t10 -c1000 inside Docker, vs Turso Cloud)

| Endpoint | req/s |
|----------|:-----:|
| Update (PATCH /links/:id) | 89,740* |
| Expand (GET /expand/:shortCode) | 46 |
| GetByID (GET /links/:id) | 17 |
| Create (POST /links) | 14 |
| Delete (DELETE /links/:id) | 9 |
| List (GET /links) | 1.0 |

> *Remote hrana to Turso Cloud: throughput is dominated by network RTT, NOT
> by the driver. These numbers measure end-to-end latency to a managed cloud
> DB and are not comparable to the local embedded variant.

## Notes

- Requires a remote Turso Cloud or Bunny database (URL + auth token).
- Mutually exclusive with `go-libsql` in the same binary (both register the
  `libsql` driver name) - see the SDK docs/database.md.
