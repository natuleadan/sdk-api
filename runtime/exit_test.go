package runtime

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/natuleadan/sdk-api/events"
)

func TestService_WithExit(t *testing.T) {
	svc := &Service{
		config:    &ServiceConfig{Name: "test", Port: 19030},
		exitFuncs: make(map[string]ExitHandler),
		exitMgr:   NewExitWorkerManager(),
	}

	svc.WithExit("onOrderConfirmed", func(ctx context.Context, msg []byte) ([]byte, error) {
		return nil, nil
	})

	if svc.exitFuncs["onOrderConfirmed"] == nil {
		t.Error("exit handler not registered")
	}
}

func TestExitWorker_Validation(t *testing.T) {
	w := ExitWorker{Name: "bad-worker"}
	err := w.Validate()
	if err == nil {
		t.Error("expected error for missing subscribe.stream")
	}
}

func TestExitWorker_Defaults(t *testing.T) {
	w := ExitWorker{
		Name:      "valid",
		Subscribe: SubscribeDef{Stream: "orders"},
		Handler:   "onOrder",
	}
	err := w.Validate()
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if w.MaxConcurrent != 1 {
		t.Errorf("MaxConcurrent = %d, want 1", w.MaxConcurrent)
	}
	if w.Subscribe.Subject != "orders" {
		t.Errorf("Subject = %q, want orders", w.Subscribe.Subject)
	}
	if w.Subscribe.Durable != "valid-worker" {
		t.Errorf("Durable = %q, want valid-worker", w.Subscribe.Durable)
	}
}

func TestExitWorker_ReplyDefaults(t *testing.T) {
	w := ExitWorker{
		Name:      "replier",
		Subscribe: SubscribeDef{Stream: "s"},
		Handler:   "h",
		Reply:     true,
	}
	err := w.Validate()
	if err != nil {
		t.Fatal(err)
	}
	if w.ReplyTimeout != "30s" {
		t.Errorf("ReplyTimeout = %q, want 30s", w.ReplyTimeout)
	}
}

func TestExitWorkerManager_NoHandlers(t *testing.T) {
	mgr := NewExitWorkerManager()
	exitDefs := []ExitWorker{
		{Name: "test", Subscribe: SubscribeDef{Stream: "s"}, Handler: "missing"},
	}
	err := mgr.Start(context.Background(), exitDefs, nil, nil, nil)
	if err == nil {
		t.Error("expected error for nil handlers")
	}
}

func TestExitWorkerManager_Empty(t *testing.T) {
	mgr := NewExitWorkerManager()
	err := 	mgr.Start(context.Background(), nil, nil, nil, nil)
	if err != nil {
		t.Errorf("empty exit defs should not error: %v", err)
	}
	mgr.Shutdown(1 * time.Second)
}

func TestExitWorker_ShutdownTimeout(t *testing.T) {
	w := &exitWorker{
		name: "test",
		sem:  make(chan struct{}, 1),
		state: &workerState{
			shutdownCh: make(chan struct{}, 1),
		},
	}

	w.sem <- struct{}{}
	w.state.tasks.Add(1)
	go func() {
		time.Sleep(500 * time.Millisecond)
		<-w.sem
		w.state.tasks.Add(-1)
	}()
	w.shutdown(50 * time.Millisecond)
	t.Log("shutdown timeout test passed")
}

func TestExitWorkerManager_Start_NoNatsConnection(t *testing.T) {
	mgr := NewExitWorkerManager()
	exitDefs := []ExitWorker{
		{Name: "test", Subscribe: SubscribeDef{Stream: "s"}, Handler: "h"},
	}
	err := mgr.Start(context.Background(), exitDefs, map[string]events.EventBroker{}, map[string]ExitHandler{
		"h": func(ctx context.Context, msg []byte) ([]byte, error) { return nil, nil },
	}, nil)
	if err == nil {
		t.Error("expected error for no NATS connection")
	}
}

func TestExitWorkerManager_MissingHandler(t *testing.T) {
	mgr := NewExitWorkerManager()
	exitDefs := []ExitWorker{
		{Name: "test", Subscribe: SubscribeDef{Stream: "s"}, Handler: "missing"},
	}
	conn := &events.Conn{}
	conns := map[string]events.EventBroker{"default": conn}
	err := mgr.Start(context.Background(), exitDefs, conns, map[string]ExitHandler{}, nil)
	if err == nil {
		t.Error("expected error for missing handler")
	}
}

// Integration test requiring NATS — skipped in unit tests
func TestNakWithLog(t *testing.T) {
	// nakWithLog is a fire-and-forget helper; just verify it doesn't panic
	var called atomic.Bool
	m := &mockMessage{nakFn: func() error { called.Store(true); return nil }}
	nakWithLog(m, "test-worker", "test-context")
	if !called.Load() {
		t.Error("Nak not called")
	}
}

func TestExitWorker_PullBatchDefaults(t *testing.T) {
	w := ExitWorker{
		Name:          "puller",
		Subscribe:     SubscribeDef{Stream: "s", Subject: "s"},
		Handler:       "h",
		ConsumerMode:  "pull",
		MaxConcurrent: 1,
	}
	err := w.Validate()
	if err != nil {
		t.Fatal(err)
	}
	if w.PullBatch != 0 {
		t.Errorf("PullBatch = %d, want 0", w.PullBatch)
	}
}

// mockMessage implements events.Message for unit testing
type mockMessage struct {
	events.Message
	nakFn func() error
}

func (m *mockMessage) Nak(_ ...time.Duration) error {
	if m.nakFn != nil {
		return m.nakFn()
	}
	return nil
}

type mockExitHooks struct {
	onMessage func(ctx context.Context, msg []byte) ([]byte, error)
	onSuccess func(ctx context.Context)
	onError   func(ctx context.Context, err error)
}

func (m *mockExitHooks) OnMessage(ctx context.Context, msg []byte) ([]byte, error) {
	if m.onMessage != nil {
		return m.onMessage(ctx, msg)
	}
	return msg, nil
}

func (m *mockExitHooks) OnSuccess(ctx context.Context) {
	if m.onSuccess != nil {
		m.onSuccess(ctx)
	}
}

func (m *mockExitHooks) OnError(ctx context.Context, err error) {
	if m.onError != nil {
		m.onError(ctx, err)
	}
}

func TestExitWorker_Hooks(t *testing.T) {
	natsURL := os.Getenv("NATS_URL")
	if natsURL == "" {
		t.Skip("NATS_URL not set, skipping integration test")
	}

	ctx := context.Background()
	conn, err := events.Connect(ctx, events.ConnOptions{
		URL:     natsURL,
		Timeout: 3 * time.Second,
	})
	if err != nil {
		t.Skipf("NATS not available: %v", err)
	}
	defer conn.Drain()

	streamName := "hooks-test-" + fmt.Sprintf("%d", time.Now().UnixNano())
	err = conn.EnsureStream(events.StreamConfig{
		Name:     streamName,
		Subjects: []string{streamName},
	})
	if err != nil {
		t.Skipf("stream setup: %v", err)
	}

	mgr := NewExitWorkerManager()

	var hookCalls []string
	var mu sync.Mutex
	record := func(s string) {
		mu.Lock()
		hookCalls = append(hookCalls, s)
		mu.Unlock()
	}

	exitDefs := []ExitWorker{
		{
			Name:          "hook-tester",
			Subscribe:     SubscribeDef{Stream: streamName, Subject: streamName},
			Handler:       "onHook",
			MaxConcurrent: 1,
		},
	}

	err = mgr.Start(ctx, exitDefs, map[string]events.EventBroker{"default": conn}, map[string]ExitHandler{
		"onHook": func(ctx context.Context, msg []byte) ([]byte, error) {
			record("handler")
			return nil, nil
		},
	}, map[string]ExitHooks{
		"hook-tester": &mockExitHooks{
			onMessage: func(ctx context.Context, msg []byte) ([]byte, error) {
				record("onMessage")
				return msg, nil
			},
			onSuccess: func(ctx context.Context) {
				record("onSuccess")
			},
			onError: func(ctx context.Context, err error) {
				record("onError")
			},
		},
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	_, err = conn.JS.Publish(streamName, []byte(`{"test":true}`))
	if err != nil {
		t.Fatalf("publish: %v", err)
	}

	time.Sleep(time.Second)

	mu.Lock()
	calls := make([]string, len(hookCalls))
	copy(calls, hookCalls)
	mu.Unlock()

	if len(calls) < 3 {
		t.Fatalf("expected at least 3 hook calls (onMessage+handler+onSuccess), got %v", calls)
	}
	t.Logf("hook calls: %v", calls)

	mgr.Shutdown(2 * time.Second)
}

func TestExitWorker_StartStop_NATS(t *testing.T) {
	natsURL := os.Getenv("NATS_URL")
	if natsURL == "" {
		t.Skip("NATS_URL not set, skipping integration test")
	}

	ctx := context.Background()
	conn, err := events.Connect(ctx, events.ConnOptions{
		URL:           natsURL,
		MaxReconnects: 2,
		Timeout:       3 * time.Second,
	})
	if err != nil {
		t.Skipf("NATS not available: %v", err)
	}
	defer conn.Drain()

	streamName := "exit-test-" + fmt.Sprintf("%d", time.Now().UnixNano())
	err = conn.EnsureStream(events.StreamConfig{
		Name:     streamName,
		Subjects: []string{streamName},
	})
	if err != nil {
		t.Skipf("stream setup: %v", err)
	}

	mgr := NewExitWorkerManager()

	var received atomic.Int64
	exitDefs := []ExitWorker{
		{
			Name:          "receiver",
			Subscribe:     SubscribeDef{Stream: streamName, Subject: streamName},
			Handler:       "onReceive",
			MaxConcurrent: 2,
		},
	}

	err = mgr.Start(ctx, exitDefs, map[string]events.EventBroker{"default": conn}, map[string]ExitHandler{
		"onReceive": func(ctx context.Context, msg []byte) ([]byte, error) {
			received.Add(1)
			return nil, nil
		},
	}, nil)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Publish a test message
	_, err = conn.JS.Publish(streamName, []byte(`{"test":true}`))
	if err != nil {
		t.Fatalf("publish: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	if received.Load() < 1 {
		t.Error("expected at least 1 message received")
	}

	t.Logf("messages received: %d", received.Load())

	mgr.Shutdown(5 * time.Second)
}
