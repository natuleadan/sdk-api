# CLI (sdk-api)

The `sdk-api` CLI scaffolds Go microservices, generates Dockerfiles, Kubernetes manifests, and client SDKs.

## Installation

```bash
go install github.com/natuleadan/sdk-api/cmd/sdk-api@latest
```

## Commands

### `sdk-api version`

Print the SDK version.

```bash
sdk-api version
```

### `sdk-api new <name> [flags]`

Creates a new microservice scaffold with a proper project structure (handler/, logic/, svc/, config/).

```
<name>/
├── cmd/
│   └── main.go                   # Bootstrap with runtime.NewFromYAML()
├── internal/
│   ├── config/
│   │   └── config.go             # Typed config struct
│   ├── handler/
│   │   └── <resource>.go         # HTTP handler (one per resource)
│   ├── logic/
│   │   └── <resource>.go         # Business logic (pure, testable)
│   └── svc/
│       └── servicecontext.go     # DI container (pools + bulkheads)
├── models/
│   └── <model>.go                # Struct with db:"" tags + hooks
├── service.yaml
└── .env
```

**Flags:**

| Flag | Description | Default |
|------|-------------|---------|
| `--model` | Model name (PascalCase) | Service name |
| `--fields` | Comma-separated field definitions | — |
| `--port` | HTTP port | `8080` |
| `--consume` | NATS consumers: `stream:durable:handler` | — |
| `--publish` | NATS producers: `stream:after_event` | — |
| `--exit` | Exit workers: `stream:handler:name` | — |
| `--cron` | Cron jobs: `handler:name` | — |
| `--grpc` | Enable gRPC server generation | `false` |
| `--grpc-port` | gRPC server port | HTTP port + 1 |
| `--dir` | Output directory | Service name |

**Examples:**

```bash
# Basic CRUD service
sdk-api new products-svc \
    --model Product \
    --fields "name:string,price:float64,stock:int"

# With NATS and gRPC
sdk-api new orders-svc \
    --model Order \
    --fields "total:float64,status:string" \
    --consume "orders:ord-consumer:onOrderCreated" \
    --grpc

# With exit workers and cron
sdk-api new analytics-svc \
    --exit "events:onEvent:event-worker" \
    --cron "onDailyReport:daily-report"
```

### `sdk-api validate [file]`

Validates a `service.yaml` configuration against the SDK schema. Checks required fields, references, and value ranges.

```bash
sdk-api validate                      # validate service.yaml
sdk-api validate config.yaml -v       # verbose output
sdk-api validate --strict             # fail on warnings
```

| Flag | Description |
|------|-------------|
| `--verbose` | Show parsed configuration details |
| `--strict` | Exit with error on warnings |

### `sdk-api dev [flags]`

Runs the service in development mode with hot reload. Watches for file changes and automatically rebuilds and restarts.

```bash
sdk-api dev                           # watch *.go, run go run .
sdk-api dev --port 9090               # set PORT env var
sdk-api dev --cmd "make run"          # custom rebuild command
```

| Flag | Description | Default |
|------|-------------|---------|
| `--pattern` | File glob pattern to watch | `*.go` |
| `--cmd` | Command to run on change | `go run .` |
| `--port` | PORT env var for the process | — |
| `--verbose` | Log all file changes | `false` |

### `sdk-api docker [flags]`

Generates a multi-stage Dockerfile to stdout.

```bash
sdk-api docker --name myapp --port 9090 > Dockerfile
```

| Flag | Default | Description |
|------|---------|-------------|
| `--name` | `service` | Binary name |
| `--port` | `8080` | Exposed port |
| `--main` | `main.go` | Main file path |
| `--base` | `scratch` | Base image |

### `sdk-api kube [flags]`

Generates Kubernetes manifests (Deployment + Service + HPA) to stdout.

```bash
sdk-api kube --name products --image products:v1 > k8s.yaml
```

| Flag | Default | Description |
|------|---------|-------------|
| `--name` | (required) | Service name |
| `--image` | (required) | Container image |
| `--namespace` | `default` | K8s namespace |
| `--port` | `8080` | Container port |
| `--replicas` | `3` | Replicas |

Generates Deployment with resource requests/limits, ClusterIP Service, and HorizontalPodAutoscaler (CPU 80%).

### `sdk-api vercel [flags]`

Generates `vercel.json` for Vercel deployment. Validates against Vercel compatibility rules.

```bash
sdk-api vercel --output vercel.json
```

| Flag | Default | Description |
|------|---------|-------------|
| `--config` | `service.yaml` | Path to YAML config |
| `--output` | stdout | Output file path |
| `--build-command` | — | Custom build command |
| `--go-flags` | — | Extra build flags |

### `sdk-api client [flags]`

Generates typed client SDK in TypeScript, Python, Dart, Java, or Kotlin.

```bash
sdk-api client --model Product --fields "name:string,price:float64" --lang ts
```

| Flag | Description |
|------|-------------|
| `--model` | Model name (required) |
| `--fields` | Field definitions (required) |
| `--lang` | Target language: `ts`, `py`, `dart`, `java`, `kotlin` |
| `--output` | Output file path (default: stdout) |

### `sdk-api completion [bash|zsh|fish]`

Generates shell completion scripts.

```bash
source <(sdk-api completion bash)                     # bash
sdk-api completion zsh > /usr/local/share/zsh/site-functions/_sdk-api  # zsh
sdk-api completion fish > ~/.config/fish/completions/sdk-api.fish       # fish
```

## Generated Project Structure

```
my-svc/
├── cmd/
│   └── main.go                   # Bootstrap with //go:embed + runtime.NewFromYAML()
├── internal/
│   ├── config/
│   │   └── config.go             # Typed Config struct
│   ├── handler/
│   │   └── <resource>.go         # HTTP endpoints, one per resource
│   ├── logic/
│   │   └── <resource>.go         # Business logic (pure Go, testable)
│   └── svc/
│       └── servicecontext.go     # ServiceContext — dependency injection container
├── models/
│   └── model.go                  # Struct + EntryHooks
├── service.yaml                  # YAML config (embedded via //go:embed)
└── .env                          # Environment variables
```

With `--grpc`:

```
├── api/<resource>.proto           # Protobuf definition
├── grpcserver/<resource>.go       # gRPC server implementation
└── pb/<resource>.pb.go            # Generated Go structs with protobuf tags
```

The generated `cmd/main.go` uses `//go:embed` to embed `service.yaml` directly into the binary. This eliminates filesystem dependencies at runtime and works on any deployment platform (Vercel, Docker, Kubernetes, bare-metal).
