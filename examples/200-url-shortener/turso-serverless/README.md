# 200-url-shortener - turso-serverless (remote, pure Go)

URL shortener using the **tursogo-serverless** driver
(`driver: turso-serverless`) - remote Turso Cloud / Bunny over SQL-over-HTTP.
**Pure Go, no CGO, no native libraries** - ideal for containers, serverless,
and edge runtimes.

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
| Update (PATCH /links/:id) | 112,496* |
| Expand (GET /expand/:shortCode) | 47 |
| GetByID (GET /links/:id) | 16 |
| Delete (DELETE /links/:id) | 10 |
| Create (POST /links) | 11 |
| List (GET /links) | 1.4 |

> *Remote SQL-over-HTTP to Turso Cloud: throughput is dominated by network
> RTT (~1-2s per write), NOT by the driver. `Update` spikes reflect server
> connection reuse. These numbers are not comparable to the local embedded
> variant - they measure end-to-end latency to a managed cloud DB.

## Notes

- Requires a remote Turso Cloud or Bunny database (URL + auth token).
- `auth_token: "${TURSO_AUTH_TOKEN}"` in `service.yaml` resolves from env.
