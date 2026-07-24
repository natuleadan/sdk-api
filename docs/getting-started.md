# Getting Started

Create a Go microservice with auto-generated CRUD, NATS messaging, and OpenAPI docs in 5 minutes.

## Prerequisites

- Go 1.26.x
- Docker (for PostgreSQL, NATS JetStream, etc.)
- Docker Compose v2 (for running examples)

## 1. Install the CLI

```bash
go install github.com/natuleadan/sdk-api/cmd/sdk-api@latest
```

## 2. Create a microservice

```bash
sdk-api new products-svc \
    --model Product \
    --fields "name:string,price:float64,stock:int" \
    --port 8080
```

This generates:

```
products-svc/
├── cmd/
│   └── main.go                   # Bootstrap with //go:embed + runtime.NewFromYAML()
├── internal/
│   ├── config/
│   │   └── config.go             # Typed config struct
│   ├── handler/
│   │   └── products.go           # HTTP handler (one per resource)
│   ├── logic/
│   │   └── products.go           # Business logic (pure, testable)
│   └── svc/
│       └── servicecontext.go     # DI container
├── models/
│   └── product.go                # Struct with db:"" tags + hooks
├── service.yaml                  # YAML configuration (embedded in binary)
└── .env                          # Environment variables
```

## 3. Configure

Edit `service.yaml`:

```yaml
name: products-svc
port: 8080

databases:
  - name: pg-main
    driver: postgres
    url: "${DATABASE_URL}"
    pool:
      max_conns: 10
      min_conns: 2

entry:
  - type: crud
    model: Product
    db: pg-main
    table: products
    path: /products

server:
  host: "0.0.0.0"
  api_prefix: /api
  middleware:
    - path: "/api/*"
      apply:
        - logger
        - cors
  openapi:
    enabled: true
    theme: moon
```

## 4. Write hooks

Edit `models/model.go`:

```go
type Product struct {
    ID    int64   `db:"id,primary,auto" json:"id"`
    Name  string  `db:"name,required" json:"name"`
    Price float64 `db:"price" json:"price"`
    Stock int     `db:"stock,default=0" json:"stock"`
}

type ProductHooks struct {
    runtime.DefaultHooks[Product]
}

func (h *ProductHooks) BeforeCreate(ctx context.Context, req Product) (Product, error) {
    if req.Price <= 0 {
        return req, fmt.Errorf("price must be positive")
    }
    return req, nil
}
```

## 5. Wire in cmd/main.go

```go
package main

import (
    "context"
    "log"

    _ "embed"

    "products-svc/internal/svc"
    "products-svc/models"

    "github.com/natuleadan/sdk-api/db"
    "github.com/natuleadan/sdk-api/runtime"
)

//go:embed service.yaml
var configYAML []byte

func main() {
    s, err := runtime.NewFromYAML(configYAML)
    if err != nil {
        log.Fatalf("init: %v", err)
    }

    svcCtx := svc.NewServiceContext()
    pool := s.PoolPG("pg-main")
    if pool != nil {
        svcCtx.SetPool("pg-main", pool)
    }

    table, err := db.NewTable[models.Product](pool, "products")
    if err != nil {
        log.Fatalf("table: %v", err)
    }
    if err := table.AutoInit(context.Background()); err != nil {
        log.Fatalf("auto init: %v", err)
    }
    s.WithCRUD("Product",
        runtime.NewCRUDProvider(table, &models.ProductHooks{}))
    s.WithHooks("Product", &models.ProductHooks{})

    if err := s.Run(); err != nil {
        log.Fatalf("run: %v", err)
    }
}
```

## 6. Run

Start PostgreSQL:

```bash
docker run -d --name pg -p 5432:5432 \
    -e POSTGRES_USER=dev \
    -e POSTGRES_PASSWORD=devpass \
    -e POSTGRES_DB=postgres \
    postgres:18-alpine
```

Run the service:

```bash
export DATABASE_URL="postgres://dev:devpass@localhost:5432/postgres?sslmode=disable"
cd products-svc && go mod tidy && go run .
```

### Run with Docker Compose (recommended for examples)

Each example in `examples/` is self-contained:

```bash
# Functional tests only (default)
cd examples/200-url-shortener/postgres
docker compose up --abort-on-container-exit

# Functional tests + RPS benchmark
RPS_BENCH=1 docker compose up --abort-on-container-exit
```

## 7. Test

```bash
# List (empty)
curl http://localhost:8080/api/v1/products

# Create
curl -X POST http://localhost:8080/api/v1/products \
    -H "Content-Type: application/json" \
    -d '{"name":"Widget","price":9.99,"stock":100}'

# Get by ID
curl http://localhost:8080/api/v1/products/1

# Update
curl -X PATCH http://localhost:8080/api/v1/products/1 \
    -H "Content-Type: application/json" \
    -d '{"price":7.99}'

# Delete
curl -X DELETE http://localhost:8080/api/v1/products/1

# OpenAPI docs
curl http://localhost:8080/openapi.json | jq .
open http://localhost:8080/docs    # Scalar UI browser

# Health
curl http://localhost:8080/health
```

## What's next

- **Multiple databases** — Add `databases:` entries for MySQL/Turso/MongoDB
- **Event-driven workers** — Add `stream:` + `exit:` for NATS/Kafka consumers
- **Custom endpoints** — Add `type: rest` or `type: webhook` entries
- **WebSocket/SSE** — Add `type: websocket` or `type: sse` entries
- **File upload** — Add `type: file` entries with S3 storage, pool config, cache, and presigned URLs. See `examples/300-file-storage/` for 5 complete variants
- **Async jobs** — Add `type: async` entries with persistent store (`async_store.driver: postgres|redis|nats_kv`) and automatic recovery. See `examples/500-tickets/` for a full event-driven example with 45 integration tests
- **gRPC microservices** — Set `server.mode: micro` and add `grpc_server` / `grpc_clients` for inter-service communication. In monolith mode (default), gRPC is disabled.
- **Cron jobs** — Add `cron:` entries for scheduled tasks
- **Auth & security** — Add `auth:` block with JWT, API keys, roles, permissions, CSRF, and security headers. See `examples/400-auth/manual-pg/` for a complete manual auth example with 85 integration tests
- **Multi-mode deployment** — Use `--mode entry` (HTTP) vs `--mode exit` (workers)
