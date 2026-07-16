# Messaging (NATS JetStream + Kafka)

sdk-api supports two event stream brokers: **NATS JetStream** (default) and **Kafka**. The `EventBroker` interface provides a unified abstraction — endpoints can publish to any broker, and workers can consume from any broker.

## Connection

### Event streams config

```yaml
stream:
  - name: default
    driver: nats
    url: "${NATS_URL}"
    streams:
      - name: orders
        max_age: 24h
        storage: file

  - name: analytics
    driver: kafka
    brokers: ["localhost:9092"]
    consumer_group: sdk-api
    streams:
      - name: page-views
```

The `stream:` section replaces `event_streams:`. Use `stream:` with `driver: nats` or `driver: kafka`.

### Stream Default Subjects (NATS)

A NATS stream named `orders` monitors subjects `[orders, orders.>]` (wildcard for all sub-subjects).

### Kafka Topics

Kafka topics are auto-created on first publish. The `streams:` array defines declared topics for documentation, but explicit creation is handled by the broker.

## Auto-Publish (Entry → Event Stream)

Entry endpoints can auto-publish to event streams after handling a request:

```yaml
entry:
  - type: rest
    method: POST
    path: /orders
    handler: onCreateOrder
    event_stream: default                    # Publish to "default" broker
    event_publish:
      - stream: orders
        subject: order.created

  - type: crud
    resource: products
    model: Product
    event_stream: analytics                 # Publish to Kafka broker
    event_publish:
      - stream: page-views
        subject: page.view
```

The `event_publish:` field defines publish targets for event streams.

## Exit Workers (Event → Handler)

Exit workers consume from event streams and process messages:

```yaml
exit:
  - name: email-sender
    subscribe:
      stream: orders
      subject: orders.confirmed
    handler: onOrderConfirmed
    max_concurrent: 10
    event_stream: default                    # Consume from NATS

  - name: page-view-processor
    subscribe:
      stream: page-views
    handler: onPageView
    event_stream: analytics                 # Consume from Kafka
```

When `event_stream` is not specified, the first available broker is used.

## Producer API

For programmatic access, use the `events.Producer[T]` generic type:

```go
import "github.com/natuleadan/sdk-api/events"

type OrderEvent struct {
    OrderID string  `json:"order_id"`
    Amount  float64 `json:"amount"`
}

nc, _ := events.Connect(ctx, events.ConnOptions{URL: natsURL})
js, _ := nc.JS()
producer := events.NewProducer[OrderEvent](nc, js, "orders.created")

// Publish
producer.Publish(OrderEvent{OrderID: "123", Amount: 99.99})

// Publish with idempotency key
producer.PublishWithID(OrderEvent{}, "idempotency-key-123")

// Publish and wait for reply (request-reply)
reply, _ := producer.PublishAndWait(ctx, OrderEvent{}, 5*time.Second)
```

## EventBroker Interface

The `EventBroker` abstraction allows switching between NATS and Kafka without code changes:

```go
type EventBroker interface {
    Name() string
    Publish(ctx context.Context, subject string, data []byte) error
    Subscribe(ctx context.Context, subject string, durable string, handler MessageHandler) (Subscription, error)
    EnsureStream(cfg StreamConfig) error
    Close() error
}
```

Access a broker by name:

```go
broker := svc.NATS("default")
broker.Publish(ctx, "orders.created", data)
```

## Cron Publish

```yaml
cron:
  - name: daily-report
    schedule: "0 6 * * *"
    mode: nats
    publish:
      stream: cron
      subject: cron.daily-report
```

Cron jobs use the first available event broker for `mode: nats`.

## KV Store (NATS only)

NATS Key-Value store for shared state:

```go
conn := events.Connect(ctx, events.ConnOptions{...})
kv, _ := conn.EnsureKeyValue(events.DefaultKVConfig("sessions"))
kv.Put("user:123", []byte(`{"name":"Alice"}`))
```

Convenience methods (no need to import `nats.KeyValue`):

```go
val, err := conn.KVGet("sessions", "user:123")
rev, err := conn.KVPut("sessions", "user:123", []byte(`{"name":"Bob"}`))
err = conn.KVDelete("sessions", "user:123")
keys, err := conn.KVKeys("sessions")          // list all keys in bucket
err = conn.KVReset("sessions")                // delete all keys in bucket
```

Core NATS request-reply subscription (no `nats.Msg` import):

```go
// Fire-and-forget handler
conn.SubscribeRaw("orders.echo", func(data []byte) {
    log.Printf("received: %s", string(data))
})

// Echo handler (responds with the same data)
conn.SubscribeRawReply("orders.echo", func(data []byte) []byte {
    return data
})
```

Typed cache wrapper:

```go
cache := events.NewCache[User](kv, 5*time.Minute)
user, _ := cache.Get(ctx, "user:123")
cache.Set(ctx, "user:123", user)
```

## Graceful Shutdown

On shutdown, all event stream connections are drained (in-flight messages are processed before exit).
