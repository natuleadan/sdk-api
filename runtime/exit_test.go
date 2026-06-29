package runtime

import (
	"context"
	"fmt"
	"os"
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
	err := mgr.Start(context.Background(), exitDefs, map[string]*events.Conn{}, map[string]ExitHandler{
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
	conns := map[string]*events.Conn{"default": conn}
	err := mgr.Start(context.Background(), exitDefs, conns, map[string]ExitHandler{}, nil)
	if err == nil {
		t.Error("expected error for missing handler")
	}
}

// Integration test requiring NATS — skipped in unit tests
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

	err = mgr.Start(ctx, exitDefs, map[string]*events.Conn{"default": conn}, map[string]ExitHandler{
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
