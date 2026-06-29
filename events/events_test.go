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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg := DefaultConsumerConfig(name, name+"-c1")
	cfg.Subject = name
	err := ConsumePull[testEvent](ctx, conn.JS, cfg,
		func(ctx context.Context, msg Msg[testEvent]) (AckAction, error) {
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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg := DefaultConsumerConfig(name, name+"-c1")
	cfg.Subject = name
	err := ConsumePush[testEvent](ctx, conn.JS, cfg,
		func(ctx context.Context, msg Msg[testEvent]) (AckAction, error) {
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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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

	kv, err := conn.EnsureKeyValue(DefaultKVConfig(name))
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

	kv, err := conn.EnsureKeyValue(DefaultKVConfig(name))
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

func TestCacheGetOrSet(t *testing.T) {
	conn := testConn(t)
	name := testStreamName("getset")

	kv, err := conn.EnsureKeyValue(DefaultKVConfig(name))
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
