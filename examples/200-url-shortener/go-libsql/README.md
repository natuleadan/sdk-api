# 200-url-shortener - go-libsql (embedded replica)

URL shortener using the **go-libsql** driver (`driver: go-libsql`) - embedded
replicas: **local reads, writes to the cloud primary**, then reflected back.

Requires **CGO** and the `golibsql` build tag (go-libsql owns the `libsql`
driver name, mutually exclusive with libsql-client-go):

```bash
go build -tags golibsql -o svc ./cmd
```

## Run

```bash
export TURSO_DATABASE_URL="libsql://<db>.turso.io"
export TURSO_AUTH_TOKEN="<token>"
SSL_CERT_FILE=/etc/ssl/cert.pem ./svc   # CA certs required (native lib)
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

## Benchmarks (wrk -t10 -c1000 inside Docker, embedded replica vs Turso primary)

| Endpoint | req/s |
|----------|:-----:|
| Update (PATCH /links/:id) | 95,561 |
| Expand (GET /expand/:shortCode) | 68,527 |
| GetByID (GET /links/:id) | 62,425 |
| List (GET /links) | 41,852 |
| Create (POST /links) | 0.2 |
| Delete (DELETE /links/:id) | 0.2 |

> Embedded replica: **reads are local** (fast, ~40-95k req/s), **writes go to
> the Turso cloud primary** (slow, network RTT dominates). This is the
> local-first read path - ideal for low-latency reads on the same VPS.

## Notes

- Embedded replica: reads are local (fast), writes go to the Turso primary.
- Needs system CA certificates (native libSQL does its own TLS).
- Runtime image uses `debian:trixie-slim` (GLIBC 2.41) because the CGO binary
  is linked against the `golang:1.26` builder's glibc - `bookworm` (2.36) is
  too old for it. The other 3 pure-Go variants stay on `bookworm-slim`.
