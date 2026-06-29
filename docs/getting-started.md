# Getting Started

Create a Go microservice with auto-generated CRUD, NATS messaging, and OpenAPI docs in 5 minutes.

## Prerequisites

- Go 1.26+
- Docker (for PostgreSQL, NATS JetStream)
- PostgreSQL 18+

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
├── main.go              # Entrypoint: runtime.New("service.yaml")
├── service.yaml         # v2 YAML configuration
└── models/
    └── model.go         # Product struct + hooks
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
  api_prefix: /api/v1
  middleware:
    - path: "/api/v1/*"
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

## 5. Wire in main.go

```go
package main

import (
    "log"

    "products-svc/models"

    "github.com/jackc/pgx/v5/pgxpool"
    "github.com/natuleadan/sdk-api/db"
    "github.com/natuleadan/sdk-api/runtime"
)

func main() {
    svc, err := runtime.New("service.yaml")
    if err != nil {
        log.Fatalf("init: %v", err)
    }

    pgPool := svc.Pool("pg-main").(*pgxpool.Pool)
    table, err := db.NewTable[models.Product](pgPool, "products")
    if err != nil {
        log.Fatalf("table: %v", err)
    }

    svc.WithCRUD("Product",
        runtime.NewCRUDProvider(table, &models.ProductHooks{}))
    svc.RegisterModel("Product", (*models.Product)(nil))

    if err := svc.Run(); err != nil {
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

- **Multiple databases** — Add `databases:` entries for MySQL/Turso
- **NATS messaging** — Add `nats:` + `exit:` for event-driven workers
- **Custom endpoints** — Add `type: rest` or `type: webhook` entries
- **WebSocket/SSE** — Add `type: websocket` or `type: sse` entries
- **File upload** — Add `type: file` entries with storage backend
- **Cron jobs** — Add `cron:` entries for scheduled tasks
- **Multi-mode deployment** — Use `--mode entry` (HTTP) vs `--mode exit` (workers)
