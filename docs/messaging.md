# Messaging (NATS JetStream)

NATS JetStream powers all messaging in sdk-api: entry endpoints can auto-publish, exit workers consume and process, and cron jobs can publish on schedule.

## Connection

```yaml
nats:
  - name: primary
    url: "${NATS_URL}"
    max_reconnects: 10
    reconnect_wait: 2s
    timeout: 5s
    retry_on_fail: true
    streams:
      - name: orders
        max_age: 24h
        storage: file
```

Multiple NATS connections are supported. Each is referenced internally by name.

### Stream Default Subjects

A stream named `orders` automatically monitors subjects `[orders, orders.>]` (wildcard for all sub-subjects).

## Auto-Publish (Entry → NATS)

Entry endpoints (rest, webhook) can auto-publish to NATS after handling a request:

```yaml
entry:
  - type: rest
    method: POST
    path: /orders
    handler: onCreateOrder
    nats_publish:
      - stream: orders
        subject: orders.created
```

When the handler returns successfully (status < 400), the SDK publishes `c.Body()` to the subject. Errors are logged but don't fail the HTTP request.

## Exit Workers (NATS → Handler)

### Push Consumer (Default)

NATS pushes messages to the worker as they arrive:

```yaml
exit:
  - name: email-sender
    subscribe:
      stream: orders
      subject: orders.confirmed
    handler: onOrderConfirmed
    max_concurrent: 10
```

```go
svc.WithExit("onOrderConfirmed", func(ctx context.Context, msg []byte) ([]byte, error) {
    // Process message
    return nil, nil
})
```

### Pull Consumer

Worker fetches messages in batches:

```yaml
exit:
  - name: batch-worker
    subscribe:
      stream: orders
      subject: orders.batch
    handler: onBatch
    pull_batch: 10
    consumer_mode: pull           # push | pull
```

### Reply Mode

When `reply: true`, the handler's return value is sent back via `msg.Respond()`:

```yaml
exit:
  - name: order-validator
    subscribe:
      stream: orders
      subject: orders.validate
    handler: onValidate
    reply: true
    reply_timeout: 30s
```

```go
svc.WithExit("onValidate", func(ctx context.Context, msg []byte) ([]byte, error) {
    var req Request
    json.Unmarshal(msg, &req)
    result, _ := json.Marshal(Result{Valid: req.ID != ""})
    return result, nil  // SDK calls msg.Respond(result)
})
```

Publishing side (request-reply):

```go
producer := events.NewProducer[MyType](nc, js, "orders.validate")
resp, err := producer.PublishAndWait(ctx, myData, 5*time.Second)
```

## Producers

```go
producer := events.NewProducer[MyType](nc, js, "orders.created")

// Fire-and-forget
producer.Publish(event)

// Deduplicated publish
producer.PublishWithID(event, "unique-id")

// Request-reply
resp, err := producer.PublishAndWait(ctx, data, 5*time.Second)

// Raw bytes request-reply
rawResp, err := producer.PublishAndWaitRaw(ctx, []byte(`{}`), 5*time.Second)
```

## KV Cache

Typed, TTL-backed cache built on NATS KeyValue:

```go
kv, _ := conn.EnsureKeyValue(events.DefaultKVConfig("my-cache"))
cache := events.NewCache[MyType](kv, 5*time.Minute)

val, err := cache.Get(ctx, "key.1")
cache.Set(ctx, "key.1", myData)
cache.Delete(ctx, "key.1")

// Lazy initialization
val, err := cache.GetOrSet(ctx, "key.lazy", func() (*MyType, error) {
    v := computeExpensive()
    return &v, nil
})
```

Bucket naming defaults to `"events"` stream name. TTL applies per-key.

## Achieve delivery guarantees

| Mechanism | Guarantee | Configuration |
|-----------|-----------|---------------|
| Manual ack | At-least-once | `ManualAck()` in subscription |
| Nak + backoff | Retry with delay | `BackOff` in consumer config |
| Term | Stop retrying | `Term` ack action in handler |
| Dedup ID | Exactly-once | `nats.MsgId(id)` via `PublishWithID` |
