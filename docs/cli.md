# CLI (sdk-api)

The `sdk-api` CLI scaffolds Go microservices, generates Dockerfiles, Kubernetes manifests, and client SDKs.

## Installation

```bash
go install github.com/natuleadan/sdk-api/cmd/sdk-api@latest
```

## Commands

### `sdk-api version`

Print the SDK version.

### `sdk-api new <name> [flags]`

Creates a new microservice scaffold with YAML config. Generates three files:

```
<name>/
├── main.go            # Entrypoint with runtime.New()
├── service.yaml       # YAML configuration
└── models/
    └── model.go       # Struct with db:"" tags + EntryHooks
```

**Flags:**

| Flag | Description | Example |
|------|-------------|---------|
| `--model` | Model name (defaults to PascalCase of service name) | `--model Product` |
| `--fields` | Comma-separated field definitions | `--fields "name:string,price:float64"` |
| `--port` | HTTP port (default: `8080`) | `--port 9090` |
| `--consume` | NATS consumer streams | `--consume "orders:ord-cons:onOrder"` |
| `--publish` | NATS producer streams | `--publish "events:create\|update"` |
| `--exit` | Exit worker definitions | `--exit "orders:onOrderConfirmed:ord-worker"` |
| `--cron` | Cron job handlers | `--cron "onCleanup:daily-cleanup"` |
| `--dir` | Output directory (default: service name) | `--dir ./my-svc` |

**Examples:**

```bash
# Basic CRUD service
sdk-api new products-svc \
    --model Product \
    --fields "name:string,price:float64,stock:int"

# With NATS consumers and producers
sdk-api new orders-svc \
    --model Order \
    --fields "total:float64,status:string" \
    --consume "orders:ord-consumer:onOrderCreated" \
    --publish "order-events:create|update"

# With exit workers and cron
sdk-api new analytics-svc \
    --exit "events:onEvent:event-worker" \
    --cron "onDailyReport:daily-report"
```

### `sdk-api docker [flags]`

Generates a multi-stage Dockerfile to stdout.

```bash
sdk-api docker --name myapp --port 9090 > Dockerfile
```

| Flag | Default | Description |
|------|---------|-------------|
| `--name` | (required) | Binary name |
| `--port` | `8080` | Exposed port |
| `--main` | `./` | Main package path |
| `--base` | `alpine` | Runtime base image (`alpine` or `scratch`) |

### `sdk-api kube [flags]`

Generates Kubernetes manifests (Deployment + Service + HPA) to stdout.

```bash
sdk-api kube --name products --image products:v1 > k8s.yaml
```

| Flag | Default | Description |
|------|---------|-------------|
| `--name` | (required) | Service name |
| `--image` | (required) | Container image |
| `--port` | `8080` | Container port |
| `--replicas` | `3` | Initial replicas |
| `--namespace` | `default` | Kubernetes namespace |

Generates:
- Deployment with resource requests/limits
- Service (ClusterIP)
- HorizontalPodAutoscaler (target CPU: 80%)

### `sdk-api client [flags]`

Generates typed client SDK in multiple languages.

```bash
sdk-api client --model Product --fields "name:string,price:float64" --lang ts
```

| Flag | Description |
|------|-------------|
| `--model` | Model name (required) |
| `--fields` | Field definitions (required) |
| `--lang` | Target language: `ts`, `py`, `dart`, `java`, `kotlin` |
| `--output` | Output file path (default: stdout) |

## Generated Project Structure

```
my-svc/
├── main.go              # Entrypoint: runtime.New("service.yaml")
├── service.yaml         # YAML with entry/exit/cron
└── models/
    └── model.go         # Struct + EntryHooks
```

The generated `main.go` supports `--mode`:

```bash
go run . --mode entry    # HTTP server
go run . --mode exit     # NATS workers
```
