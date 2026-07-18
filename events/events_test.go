package events

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
)

var testID int32

func testStreamName(base string) string {
	n := atomic.AddInt32(&testID, 1)
	return fmt.Sprintf("%s-%d-%d", base, n, rand.Intn(99999))
}

func testConn(t *testing.T) *Conn {
	t.Helper()
	url := os.Getenv("NATS_URL")
	if url == "" {
		url = "nats://localhost:14222"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)

	conn, err := Connect(ctx, ConnOptions{URL: url, RetryOnFail: true})
	if err != nil {
		t.Skipf("NATS not available at %s: %v", url, err)
	}
	t.Cleanup(conn.Drain)
	return conn
}

func testStream(t *testing.T, conn *Conn, base string) string {
	t.Helper()
	name := testStreamName(base)
	err := conn.EnsureStream(DefaultStreamConfig(name))
	if err != nil {
		t.Fatalf("EnsureStream(%s): %v", name, err)
	}
	return name
}

func testKVConfig(name string) KVConfig {
	cfg := DefaultKVConfig(name)
	cfg.Storage = FileStorage
	return cfg
}

type testEvent struct {
	ID      int    `json:"id"`
	Payload string `json:"payload"`
}

func TestConnectAndStream(t *testing.T) {
	conn := testConn(t)
	name := testStream(t, conn, "connect")

	err := conn.EnsureStream(DefaultStreamConfig(name))
	if err != nil {
		t.Fatalf("EnsureStream idempotent: %v", err)
	}
}

func TestProducerPublish(t *testing.T) {
	conn := testConn(t)
	name := testStream(t, conn, "producer")

	producer := NewProducer[testEvent](conn.NC, conn.JS, name)
	err := producer.Publish(testEvent{ID: 1, Payload: "hello"})
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
}

func TestProducerPublishWithID(t *testing.T) {
	conn := testConn(t)
	name := testStream(t, conn, "producer-id")

	producer := NewProducer[testEvent](conn.NC, conn.JS, name)
	err := producer.PublishWithID(testEvent{ID: 1, Payload: "dedup"}, "evt-1")
	if err != nil {
		t.Fatalf("PublishWithID: %v", err)
	}

	err = producer.PublishWithID(testEvent{ID: 1, Payload: "dedup"}, "evt-1")
	if err != nil {
		t.Fatalf("PublishWithID duplicate (should succeed): %v", err)
	}
}

func TestConsumePull(t *testing.T) {
	conn := testConn(t)
	name := testStream(t, conn, "consume-pull")

	producer := NewProducer[testEvent](conn.NC, conn.JS, name)
	for i := range 3 {
		producer.Publish(testEvent{ID: i, Payload: "pull-test"})
	}
	time.Sleep(500 * time.Millisecond)

	var received atomic.Int32
	ctx := t.Context()

	cfg := DefaultConsumerConfig(name, name+"-c1")
	cfg.Subject = name
	err := ConsumePull[testEvent](ctx, conn.JS, cfg,
		func(_ context.Context, msg Msg[testEvent]) (AckAction, error) {
			received.Add(1)
			return Ack, nil
		})
	if err != nil {
		t.Fatalf("ConsumePull: %v", err)
	}

	time.Sleep(2 * time.Second)

	if n := received.Load(); n != 3 {
		t.Errorf("expected 3 messages, got %d", n)
	}
}

func TestConsumePush(t *testing.T) {
	conn := testConn(t)
	name := testStream(t, conn, "consume-push")

	producer := NewProducer[testEvent](conn.NC, conn.JS, name)
	for i := range 3 {
		producer.Publish(testEvent{ID: i, Payload: "push-test"})
	}
	time.Sleep(500 * time.Millisecond)

	var received atomic.Int32
	ctx := t.Context()

	cfg := DefaultConsumerConfig(name, name+"-c1")
	cfg.Subject = name
	err := ConsumePush[testEvent](ctx, conn.JS, cfg,
		func(_ context.Context, msg Msg[testEvent]) (AckAction, error) {
			received.Add(1)
			return Ack, nil
		})
	if err != nil {
		t.Fatalf("ConsumePush: %v", err)
	}

	time.Sleep(2 * time.Second)

	if n := received.Load(); n != 3 {
		t.Errorf("expected 3 messages, got %d", n)
	}
}

func TestConsumeNakThenAck(t *testing.T) {
	conn := testConn(t)
	name := testStream(t, conn, "nak-ack")

	var count atomic.Int32
	ctx := t.Context()

	cfg := DefaultConsumerConfig(name, name+"-c1")
	cfg.Subject = name
	cfg.MaxDeliver = 10
	err := ConsumePush[testEvent](ctx, conn.JS, cfg,
		func(ctx context.Context, msg Msg[testEvent]) (AckAction, error) {
			n := count.Add(1)
			if n <= 1 {
				return Nak, nil
			}
			return Ack, nil
		})
	if err != nil {
		t.Fatalf("ConsumePush: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	producer := NewProducer[testEvent](conn.NC, conn.JS, name)
	producer.Publish(testEvent{ID: 1, Payload: "nak-test"})

	time.Sleep(2 * time.Second)

	n := count.Load()
	if n < 2 {
		t.Errorf("expected at least 2 (1 nak + 1 ack), got %d", n)
	}
}

func TestConsumeTerm(t *testing.T) {
	conn := testConn(t)
	name := testStream(t, conn, "term")

	ctx := t.Context()

	cfg := DefaultConsumerConfig(name, name+"-c1")
	cfg.Subject = name
	err := ConsumePush[testEvent](ctx, conn.JS, cfg,
		func(ctx context.Context, msg Msg[testEvent]) (AckAction, error) {
			return Term, nil
		})
	if err != nil {
		t.Fatalf("ConsumePush: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	producer := NewProducer[testEvent](conn.NC, conn.JS, name)
	producer.Publish(testEvent{ID: 1, Payload: "term-test"})

	time.Sleep(2 * time.Second)
}

func TestConsumerJSONError(t *testing.T) {
	conn := testConn(t)
	name := testStream(t, conn, "json-err")

	ctx := t.Context()

	cfg := DefaultConsumerConfig(name, name+"-c1")
	cfg.Subject = name
	err := ConsumePush[testEvent](ctx, conn.JS, cfg,
		func(ctx context.Context, msg Msg[testEvent]) (AckAction, error) {
			return Ack, nil
		})
	if err != nil {
		t.Fatalf("ConsumePush: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	conn.JS.Publish(name, []byte("not-json"))

	time.Sleep(2 * time.Second)
}

func TestConsumeQueueGroup(t *testing.T) {
	conn := testConn(t)
	name := testStream(t, conn, "queue-group")

	producer := NewProducer[testEvent](conn.NC, conn.JS, name)
	for i := range 5 {
		producer.Publish(testEvent{ID: i, Payload: "queue-test"})
	}
	time.Sleep(500 * time.Millisecond)

	var total atomic.Int32
	ctx := t.Context()

	cfg := DefaultConsumerConfig(name, name+"-durable")
	cfg.Subject = name
	cfg.QueueGroup = name + "-workers"

	err := ConsumePush[testEvent](ctx, conn.JS, cfg,
		func(ctx context.Context, msg Msg[testEvent]) (AckAction, error) {
			total.Add(1)
			return Ack, nil
		})
	if err != nil {
		t.Fatalf("ConsumePush: %v", err)
	}

	time.Sleep(2 * time.Second)

	if n := total.Load(); n != 5 {
		t.Errorf("expected 5 messages, got %d", n)
	}
}

func TestWaitForNATS(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	url := os.Getenv("NATS_URL")
	if url == "" {
		url = "nats://localhost:14222"
	}

	err := WaitForNATS(ctx, url, 3, 200*time.Millisecond)
	if err != nil {
		t.Skipf("WaitForNATS: %v", err)
	}
}

func TestKV(t *testing.T) {
	conn := testConn(t)
	name := testStreamName("kv")

	kv, err := conn.EnsureKeyValue(testKVConfig(name))
	if err != nil {
		t.Fatalf("EnsureKeyValue: %v", err)
	}

	// Set
	_, err = kv.Put("hello", []byte("world"))
	if err != nil {
		t.Fatalf("Put: %v", err)
	}

	// Get
	entry, err := kv.Get("hello")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(entry.Value()) != "world" {
		t.Errorf("expected 'world', got %q", entry.Value())
	}

	// Delete
	err = kv.Delete("hello")
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	_, err = kv.Get("hello")
	if err != nats.ErrKeyNotFound {
		t.Errorf("expected ErrKeyNotFound after delete, got %v", err)
	}
}

func TestCacheTyped(t *testing.T) {
	conn := testConn(t)
	name := testStreamName("cache")

	kv, err := conn.EnsureKeyValue(testKVConfig(name))
	if err != nil {
		t.Fatalf("EnsureKeyValue: %v", err)
	}

	type testData struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}

	cache := NewCache[testData](kv, 5*time.Minute)
	ctx := context.Background()

	err = cache.Set(ctx, "obj.1", testData{ID: 1, Name: "alice"})
	if err != nil {
		t.Fatalf("Cache.Set: %v", err)
	}

	got, err := cache.Get(ctx, "obj.1")
	if err != nil {
		t.Fatalf("Cache.Get: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil")
	}
	if got.ID != 1 || got.Name != "alice" {
		t.Errorf("expected {1 alice}, got {%d %s}", got.ID, got.Name)
	}

	cache.Delete(ctx, "obj.1")

	got, err = cache.Get(ctx, "obj.1")
	if err != nil {
		t.Fatalf("Cache.Get after delete: %v", err)
	}
	if got != nil {
		t.Error("expected nil after delete")
	}
}

func TestSubscribeRaw(t *testing.T) {
	conn := testConn(t)
	subject := "test.raw." + testStreamName("raw")

	var received atomic.Int32
	err := conn.SubscribeRaw(subject, func(msg []byte) {
		received.Add(1)
	})
	if err != nil {
		t.Fatalf("SubscribeRaw: %v", err)
	}

	conn.NC.Publish(subject, []byte("hello"))
	time.Sleep(500 * time.Millisecond)

	if received.Load() < 1 {
		t.Error("expected at least 1 message via SubscribeRaw")
	}
}

func TestSubscribeRawReply(t *testing.T) {
	conn := testConn(t)
	subject := "test.rr." + testStreamName("rawrr")

	err := conn.SubscribeRawReply(subject, func(msg []byte) []byte {
		return append([]byte("echo:"), msg...)
	})
	if err != nil {
		t.Fatalf("SubscribeRawReply: %v", err)
	}

	resp, err := conn.NC.Request(subject, []byte("ping"), 3*time.Second)
	if err != nil {
		t.Fatalf("Request: %v", err)
	}
	if string(resp.Data) != "echo:ping" {
		t.Errorf("expected 'echo:ping', got %q", resp.Data)
	}
}

func TestConnName(t *testing.T) {
	ctx := context.Background()
	conn, err := Connect(ctx, ConnOptions{URL: os.Getenv("NATS_URL"), Name: "my-test-conn"})
	if err != nil {
		t.Skipf("NATS not available: %v", err)
	}
	defer conn.Drain()
	if conn.Name() != "my-test-conn" {
		t.Errorf("Name() = %q, want 'my-test-conn'", conn.Name())
	}
}

func TestConnContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	conn, err := Connect(ctx, ConnOptions{URL: os.Getenv("NATS_URL")})
	if err != nil {
		t.Skipf("NATS not available: %v", err)
	}
	defer conn.Drain()
	if err := conn.Context().Err(); err == nil {
		t.Error("expected cancelled context")
	}
}

func TestConnClose(t *testing.T) {
	conn := testConn(t)
	if err := conn.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
	// second Close should be safe
	if err := conn.Close(); err != nil {
		t.Errorf("Close (second): %v", err)
	}
}

func TestConnPublishJSON(t *testing.T) {
	conn := testConn(t)
	name := testStream(t, conn, "pubjson")

	err := conn.PublishJSON(context.Background(), name, testEvent{ID: 42, Payload: "json-test"})
	if err != nil {
		t.Fatalf("PublishJSON: %v", err)
	}

	var received atomic.Int32
	cfg := DefaultConsumerConfig(name, name+"-c1")
	cfg.Subject = name
	err = ConsumePull[testEvent](context.Background(), conn.JS, cfg,
		func(ctx context.Context, msg Msg[testEvent]) (AckAction, error) {
			if msg.Data.ID == 42 {
				received.Add(1)
			}
			return Ack, nil
		})
	if err != nil {
		t.Fatalf("ConsumePull: %v", err)
	}
	time.Sleep(2 * time.Second)

	if received.Load() < 1 {
		t.Error("PublishJSON message not received")
	}
}

func TestConnRequest(t *testing.T) {
	conn := testConn(t)
	subject := "conn.req." + testStreamName("req")

	conn.NC.Subscribe(subject, func(m *nats.Msg) {
		m.Respond([]byte("pong"))
	})
	conn.NC.Flush()

	resp, err := conn.Request(context.Background(), subject, []byte("ping"), 3*time.Second)
	if err != nil {
		t.Fatalf("Conn.Request: %v", err)
	}
	if string(resp) != "pong" {
		t.Errorf("expected 'pong', got %q", resp)
	}
}

func TestConnEnsureStreams(t *testing.T) {
	conn := testConn(t)
	name1 := testStreamName("es1")
	name2 := testStreamName("es2")

	err := conn.EnsureStreams(
		DefaultStreamConfig(name1),
		DefaultStreamConfig(name2),
	)
	if err != nil {
		t.Fatalf("EnsureStreams: %v", err)
	}

	if err := conn.EnsureStreams(); err != nil {
		t.Errorf("EnsureStreams (empty): %v", err)
	}
}

func TestConnKVGet(t *testing.T) {
	conn := testConn(t)
	name := testStreamName("kvget")

	rev, err := conn.KVPut(name, "k", []byte("val"))
	if err != nil {
		t.Skipf("KVPut: %v (server may not support KV)", err)
	}
	if rev == 0 {
		t.Error("expected non-zero revision")
	}

	data, err := conn.KVGet(name, "k")
	if err != nil {
		t.Fatalf("KVGet: %v", err)
	}
	if string(data) != "val" {
		t.Errorf("expected 'val', got %q", data)
	}
}

func TestConnKVGetNonExistent(t *testing.T) {
	conn := testConn(t)
	name := testStreamName("kvgne")

	_, err := conn.KVGet(name, "nonexistent")
	if err == nil {
		t.Fatal("expected error for non-existent key")
	}
}

func TestConnKVDelete(t *testing.T) {
	conn := testConn(t)
	name := testStreamName("kvdel")

	_, err := conn.KVPut(name, "del", []byte("bye"))
	if err != nil {
		t.Skipf("KVPut: %v (server may not support KV)", err)
	}

	if err := conn.KVDelete(name, "del"); err != nil {
		t.Fatalf("KVDelete: %v", err)
	}
}

func TestConnKVDeleteNonExistent(t *testing.T) {
	conn := testConn(t)
	name := testStreamName("kvdne")

	err := conn.KVDelete(name, "never-existed")
	if err != nil {
		t.Errorf("expected no error deleting non-existent key, got %v", err)
	}
}

func TestConnKVKeys(t *testing.T) {
	conn := testConn(t)
	name := testStreamName("kvkeys")

	_, err := conn.KVPut(name, "k1", []byte("v1"))
	if err != nil {
		t.Skipf("KVPut: %v (server may not support KV)", err)
	}
	conn.KVPut(name, "k2", []byte("v2"))
	conn.KVPut(name, "k3", []byte("v3"))

	keys, err := conn.KVKeys(name)
	if err != nil {
		t.Fatalf("KVKeys: %v", err)
	}
	if len(keys) != 3 {
		t.Errorf("expected 3 keys, got %d: %v", len(keys), keys)
	}
}

func TestConnKVKeysEmpty(t *testing.T) {
	conn := testConn(t)
	name := testStreamName("kvkeysempty")

	keys, err := conn.KVKeys(name)
	if err != nil {
		t.Skipf("KVKeys: %v (server may not support KV)", err)
	}
	if len(keys) != 0 {
		t.Errorf("expected 0 keys, got %d", len(keys))
	}
}

func TestConnKVReset(t *testing.T) {
	conn := testConn(t)
	name := testStreamName("kvreset")

	_, err := conn.KVPut(name, "a", []byte("1"))
	if err != nil {
		t.Skipf("KVPut: %v (server may not support KV)", err)
	}
	conn.KVPut(name, "b", []byte("2"))
	conn.KVPut(name, "c", []byte("3"))

	if err := conn.KVReset(name); err != nil {
		t.Fatalf("KVReset: %v", err)
	}

	keys, err := conn.KVKeys(name)
	if err != nil {
		t.Fatalf("KVKeys after reset: %v", err)
	}
	if len(keys) != 0 {
		t.Errorf("expected 0 keys after reset, got %d: %v", len(keys), keys)
	}
}

func TestConnectInvalidURL(t *testing.T) {
	ctx := context.Background()
	_, err := Connect(ctx, ConnOptions{URL: "nats://invalid.local:14222", Timeout: time.Second})
	if err == nil {
		t.Error("expected error for invalid NATS URL")
	}
}

func TestConsumeNakDelay(t *testing.T) {
	conn := testConn(t)
	name := testStream(t, conn, "nakdelay")

	var count atomic.Int32
	ctx := t.Context()

	cfg := DefaultConsumerConfig(name, name+"-c1")
	cfg.Subject = name
	cfg.MaxDeliver = 10
	cfg.NakDelay = time.Second
	err := ConsumePush[testEvent](ctx, conn.JS, cfg,
		func(ctx context.Context, msg Msg[testEvent]) (AckAction, error) {
			n := count.Add(1)
			if n <= 1 {
				return NakDelay, nil
			}
			return Ack, nil
		})
	if err != nil {
		t.Fatalf("ConsumePush: %v", err)
	}
	time.Sleep(500 * time.Millisecond)

	producer := NewProducer[testEvent](conn.NC, conn.JS, name)
	producer.Publish(testEvent{ID: 1, Payload: "nakdelay-test"})
	time.Sleep(4 * time.Second)

	n := count.Load()
	if n < 2 {
		t.Errorf("expected at least 2 (1 NakDelay + 1 Ack), got %d", n)
	}
}

func TestConsumeHandlerError(t *testing.T) {
	conn := testConn(t)
	name := testStream(t, conn, "handlerr")

	var count atomic.Int32
	ctx := t.Context()

	cfg := DefaultConsumerConfig(name, name+"-c1")
	cfg.Subject = name
	cfg.MaxDeliver = 10
	err := ConsumePush[testEvent](ctx, conn.JS, cfg,
		func(ctx context.Context, msg Msg[testEvent]) (AckAction, error) {
			n := count.Add(1)
			if n <= 1 {
				return Ack, fmt.Errorf("simulated handler error")
			}
			return Ack, nil
		})
	if err != nil {
		t.Fatalf("ConsumePush: %v", err)
	}
	time.Sleep(500 * time.Millisecond)

	producer := NewProducer[testEvent](conn.NC, conn.JS, name)
	producer.Publish(testEvent{ID: 1, Payload: "handlerr-test"})
	time.Sleep(4 * time.Second)

	n := count.Load()
	if n < 2 {
		t.Errorf("expected at least 2 (1 err + 1 retry), got %d", n)
	}
}

func TestConsumeDefaultAck(t *testing.T) {
	conn := testConn(t)
	name := testStream(t, conn, "defack")

	var received atomic.Int32
	ctx := t.Context()

	cfg := DefaultConsumerConfig(name, name+"-c1")
	cfg.Subject = name
	err := ConsumePush[testEvent](ctx, conn.JS, cfg,
		func(ctx context.Context, msg Msg[testEvent]) (AckAction, error) {
			received.Add(1)
			return AckAction(99), nil // unknown action → default Ack
		})
	if err != nil {
		t.Fatalf("ConsumePush: %v", err)
	}
	time.Sleep(500 * time.Millisecond)

	producer := NewProducer[testEvent](conn.NC, conn.JS, name)
	producer.Publish(testEvent{ID: 1, Payload: "defack-test"})
	time.Sleep(2 * time.Second)

	if received.Load() < 1 {
		t.Error("expected message to be received and default-acked")
	}
}

func TestNatsMessageRespond(t *testing.T) {
	conn := testConn(t)
	subject := "natsmsg.respond." + testStreamName("respond")

	conn.NC.Subscribe(subject, func(m *nats.Msg) {
		nm := &natsMessage{msg: m}
		nm.Respond([]byte("reply-ok"))
	})
	conn.NC.Flush()

	resp, err := conn.NC.Request(subject, []byte("req"), 3*time.Second)
	if err != nil {
		t.Fatalf("Request: %v", err)
	}
	if string(resp.Data) != "reply-ok" {
		t.Errorf("expected 'reply-ok', got %q", resp.Data)
	}
}

func TestCacheGetNilValue(t *testing.T) {
	conn := testConn(t)
	name := testStreamName("cachenil")

	kv, err := conn.EnsureKeyValue(testKVConfig(name))
	if err != nil {
		t.Skipf("EnsureKeyValue: %v", err)
	}

	cache := NewCache[string](kv, 5*time.Minute)
	ctx := context.Background()

	got, err := cache.Get(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("Get non-existent: %v", err)
	}
	if got != nil {
		t.Error("expected nil for non-existent key")
	}
}

func TestCacheGetOrSetFnReturnsNil(t *testing.T) {
	conn := testConn(t)
	name := testStreamName("cachenilfn")

	kv, err := conn.EnsureKeyValue(testKVConfig(name))
	if err != nil {
		t.Skipf("EnsureKeyValue: %v", err)
	}

	cache := NewCache[string](kv, 5*time.Minute)
	ctx := context.Background()

	val, err := cache.GetOrSet(ctx, "nilkey", func() (*string, error) {
		return nil, nil
	})
	if err != nil {
		t.Fatalf("GetOrSet: %v", err)
	}
	if val != nil {
		t.Error("expected nil when fn returns nil")
	}
}

func TestNatsSubscriptionUnsubscribe(t *testing.T) {
	conn := testConn(t)
	name := testStream(t, conn, "subunsub")

	sub, err := conn.JS.Subscribe(name, func(m *nats.Msg) { m.Ack() }, nats.ManualAck())
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	ns := &natsSubscription{sub: sub}
	if err := ns.Unsubscribe(); err != nil {
		t.Errorf("Unsubscribe: %v", err)
	}
}

func TestEnsureKeyValueIdempotent(t *testing.T) {
	conn := testConn(t)
	name := testStreamName("kvido")

	kv1, err := conn.EnsureKeyValue(testKVConfig(name))
	if err != nil {
		t.Skipf("EnsureKeyValue: %v (server may not support KV)", err)
	}

	kv2, err := conn.EnsureKeyValue(testKVConfig(name))
	if err != nil {
		t.Fatalf("EnsureKeyValue (second): %v", err)
	}

	if kv1 != kv2 {
		t.Error("expected same KV store instance from cache")
	}
}

func TestCacheGetOrSet(t *testing.T) {
	conn := testConn(t)
	name := testStreamName("getset")

	kv, err := conn.EnsureKeyValue(testKVConfig(name))
	if err != nil {
		t.Fatalf("EnsureKeyValue: %v", err)
	}

	cache := NewCache[string](kv, 5*time.Minute)
	ctx := context.Background()

	var calls int
	val, err := cache.GetOrSet(ctx, "key.lazy", func() (*string, error) {
		calls++
		v := "computed"
		return &v, nil
	})
	if err != nil {
		t.Fatalf("GetOrSet: %v", err)
	}
	if *val != "computed" {
		t.Errorf("expected 'computed', got %q", *val)
	}
	if calls != 1 {
		t.Errorf("expected 1 call to fn, got %d", calls)
	}

	val2, err := cache.GetOrSet(ctx, "key.lazy", func() (*string, error) {
		calls++
		v := "should not be called"
		return &v, nil
	})
	if err != nil {
		t.Fatalf("GetOrSet second: %v", err)
	}
	if *val2 != "computed" {
		t.Errorf("expected 'computed' from cache, got %q", *val2)
	}
	if calls != 1 {
		t.Errorf("expected fn still 1 call (cached), got %d", calls)
	}
}
