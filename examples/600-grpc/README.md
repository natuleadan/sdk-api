# 600-grpc — gRPC Microservices Demo

Demonstrates inter-service communication using gRPC in micro mode. Eight services work together to process money transfers across accounts, with NATS event streaming for async processing and PostgreSQL persistence.

## Architecture

```
                        ┌──────────────────────────────────────┐
                        │              NATS                     │
                        │  transfers.initiated -> account-svc   │
                        │  transfers.completed -> transfer-svc  │
                        └──────┬───────────────────────────────┘
                               │
 ┌──────────┐   gRPC     ┌─────▼──────┐   gRPC     ┌──────────────┐
 │ url-svc  │───────────►│  auth-svc  │           │ account-svc   │
 │ file-svc │───────────►│  (:50051)  │           │  (:50055)     │
 │ticket-svc│───────────►│ DeductCr.  │           │ DeductBalance │
 └──────────┘            └────────────┘           │ CreditBalance  │
                                                  └──────┬───────┘
                               ┌──────────────────────────┘
                               │ gRPC (DeductBalance)
                     ┌─────────▼──────────┐     ┌──────────────┐
                     │   transfer-svc      │────►│  fraud-svc   │
                     │   (:50056)          │ HTTP│  (threshold) │
                     │ POST /transfers     │     └──────────────┘
                     │ → gRPC: deduct      │
                     │ → NATS: initiated    │     ┌──────────────┐
                     │ ← NATS: completed   │     │ receipt-svc  │
                     └────────────────────┘     │  (stub)      │
                                                └──────────────┘
```

## Service Map

| Service | Port | gRPC Server | gRPC Clients | DB | Auth | Key Role |
|---------|------|-------------|--------------|----|------|----------|
| **auth-svc** | 23601 | :50051 | — | postgres | jwt | DeductCredit, VerifyToken |
| **url-svc** | 23602 | — | auth-svc | postgres | jwt | Create/expand short links |
| **file-svc** | 23603 | — | auth-svc | postgres | jwt | Upload/download files |
| **ticket-svc** | 23604 | — | auth-svc | postgres | jwt | Buy tickets |
| **account-svc** | 23605 | :50055 | — | postgres | jwt | DeductCredit, CreditBalance, events |
| **transfer-svc** | 23606 | :50056 | account-svc | postgres | jwt | Initiate transfers, NATS events |
| **fraud-svc** | 23607 | — | — | — | — | Threshold fraud check (HTTP) |
| **receipt-svc** | 23608 | — | — | postgres | jwt | Receipt stub (HTTP) |

## Data Flow — Transfer Lifecycle

```
1. POST /transfers (transfer-svc)
   ├── gRPC -> account-svc.DeductBalance()
   ├── INSERT transfer (status: initiated)
   └── NATS: transfers.initiated

2. NATS consumer (account-svc)
   ├── UPDATE accounts (deduct from, credit to)
   ├── INSERT transaction record
   └── NATS: transfers.completed

3. NATS consumer (transfer-svc)
   └── UPDATE transfer (status: completed)
```

## Config Example

```yaml
# service.yaml for a gRPC server
name: my-svc
port: 23601
server:
  mode: micro
  grpc_server:
    listen_on: ":50051"
    health: true

# service.yaml for a gRPC client
server:
  mode: micro
  grpc_clients:
    - name: auth-svc
      target: direct:///auth-svc:50055
```

## Running

```bash
# Build and start all services (one at a time)
./deploy.sh

# Or manually in order:
docker compose build auth-svc && docker compose up -d auth-svc
docker compose build url-svc && docker compose up -d url-svc
docker compose build file-svc && docker compose up -d file-svc
docker compose build ticket-svc && docker compose up -d ticket-svc
docker compose build account-svc && docker compose up -d account-svc
docker compose build transfer-svc && docker compose up -d transfer-svc
docker compose build fraud-svc && docker compose up -d fraud-svc
docker compose build receipt-svc && docker compose up -d receipt-svc

# Run integration tests
docker compose run --rm bench
```

## SDK Helpers Used

| Helper | Location | Purpose |
|--------|----------|---------|
| `runtime.MustGetGrpcServer` | `runtime/grpc.go` | Get gRPC server or panic |
| `runtime.MustGetGrpcClientConn` | `runtime/grpc.go` | Get gRPC client conn or nil |
| `runtime.GrpcCall[T]` | `runtime/grpc.go` | Typed gRPC call with nil guard |
| `runtime.PGPool` | `runtime/service.go` | Type alias for pgxpool.Pool |
| `runtime.ClientConnInterface` | `runtime/grpc.go` | Type alias for grpc.ClientConnInterface |
