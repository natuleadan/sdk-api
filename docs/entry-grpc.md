# gRPC Entry Type

The `type: grpc` entry wires a protobuf service onto the runtime gRPC server. It is paired with `svc.RegisterGrpcService()` which provides the proto server implementation. gRPC is available only in `server.mode: micro`.

## Configuration

```yaml
server:
  mode: micro
  grpc_server:
    listen_on: ":50051"
    health: true

entry:
  - type: grpc
    service_name: AccountService
```

| Field | Description |
|-------|-------------|
| `service_name` | gRPC proto service name (required; maps to `RegisterGrpcService`) |

## Registration

Register the proto server implementation with the same service name declared in YAML:

```go
svc.RegisterGrpcService("AccountService", func(srv *grpc.Server) {
    accountpb.RegisterAccountServiceServer(srv, server.NewAccountGRPCServer(pool))
})
```

The SDK auto-wires the server onto the runtime gRPC server (with interceptors: trace, breaker, timeout, adaptive shedding, panic recovery, Prometheus metrics) before it starts serving.

## Client Calls

Services that consume a gRPC service configure a client in YAML:

```yaml
server:
  mode: micro
  grpc_clients:
    - name: auth-svc
      target: direct:///auth-svc:50051
```

Call it from a handler with `runtime.GrpcCall`, which handles the nil client guard:

```go
cr, err := runtime.GrpcCall(ctx, svc.GetGRPCClient("auth-svc"),
    func(conn runtime.ClientConnInterface) (*authpb.DeductCreditResponse, error) {
        return authpb.NewAuthServiceClient(conn).DeductCredit(ctx, req)
    })
if err != nil || !cr.Ok {
    return c.Status(402).JSON(runtime.Map{"error": "insufficient credits"})
}
```

## Helpers

| Helper | Purpose |
|--------|---------|
| `svc.RegisterGrpcService(name, fn)` | Register a proto server factory by service name |
| `runtime.MustGetGrpcServer(svc)` | Get `*grpc.Server` or panic (micro mode only) |
| `runtime.MustGetGrpcClientConn(svc, name)` | Get client connection or nil |
| `runtime.GrpcCall[T](ctx, gc, fn)` | Typed gRPC call with nil guard |
| `runtime.PGPool` | Type alias for `*pgxpool.Pool` |
| `runtime.ClientConnInterface` | Type alias for `grpc.ClientConnInterface` |

## CLI Scaffold

```bash
sdk-api new my-svc --grpc --grpc-service MyService
```

Generates `service.yaml` with `type: grpc` entry, `grpc_server` config, proto file, pb stubs, and a gRPC server implementation wired via `RegisterGrpcService`.

## Example

See `examples/600-grpc/` for 8 connected microservices using gRPC + NATS with 153 integration tests.
