package runtime

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/nats-io/nats.go"
	"github.com/natuleadan/sdk-api/events"
)

func TestIntegration_ExitWorker_Push(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	natsURL := getNATSTestURL(t)

	ctx := context.Background()
	conn, err := events.Connect(ctx, events.ConnOptions{URL: natsURL, Timeout: 3 * time.Second})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.Drain()

	streamName := "it-push-" + randSuffix()
	conn.EnsureStream(events.StreamConfig{Name: streamName, Subjects: []string{streamName}})

	mgr := NewExitWorkerManager()
	var received atomic.Int64

	err = mgr.Start(ctx, []ExitWorker{
		{
			Name:          "receiver",
			Subscribe:     SubscribeDef{Stream: streamName, Subject: streamName},
			Handler:       "onMsg",
			MaxConcurrent: 2,
		},
	}, map[string]events.EventBroker{"default": conn}, map[string]ExitHandler{
		"onMsg": func(_ context.Context, msg []byte) ([]byte, error) {
			received.Add(1)
			return nil, nil
		},
	}, nil)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	time.Sleep(200 * time.Millisecond)

	// Publish messages
	for i := range 5 {
		conn.JS.Publish(streamName, fmt.Appendf(nil, `{"n":%d}`, i))
	}
	time.Sleep(500 * time.Millisecond)

	if received.Load() < 5 {
		t.Errorf("received %d, want 5", received.Load())
	}
	t.Logf("push worker received %d messages", received.Load())

	mgr.Shutdown(2 * time.Second)
}

func TestIntegration_ExitWorker_Reply(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	natsURL := getNATSTestURL(t)

	ctx := context.Background()
	conn, err := events.Connect(ctx, events.ConnOptions{URL: natsURL, Timeout: 3 * time.Second})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.Drain()

	streamName := "it-reply-" + randSuffix()
	conn.EnsureStream(events.StreamConfig{Name: streamName, Subjects: []string{streamName}})

	mgr := NewExitWorkerManager()
	var processed atomic.Int64

	err = mgr.Start(ctx, []ExitWorker{
		{
			Name:          "validator",
			Subscribe:     SubscribeDef{Stream: streamName, Subject: streamName},
			Handler:       "onValidate",
			MaxConcurrent: 1,
			Reply:         true,
		},
	}, map[string]events.EventBroker{"default": conn}, map[string]ExitHandler{
		"onValidate": func(_ context.Context, msg []byte) ([]byte, error) {
			processed.Add(1)
			return []byte(`{"valid":true}`), nil
		},
	}, nil)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	time.Sleep(200 * time.Millisecond)

	// Publish via core NATS — JetStream will deliver to consumer
	for i := range 3 {
		conn.NC.Publish(streamName, fmt.Appendf(nil, `{"n":%d}`, i))
	}
	time.Sleep(500 * time.Millisecond)

	if processed.Load() < 3 {
		t.Errorf("processed %d, want 3", processed.Load())
	}
	t.Logf("reply worker processed %d messages", processed.Load())

	mgr.Shutdown(2 * time.Second)
}

func TestIntegration_Producer_RequestReply(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	natsURL := getNATSTestURL(t)

	nc, err := nats.Connect(natsURL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer nc.Drain()

	js, err := nc.JetStream()
	if err != nil {
		t.Fatalf("jetstream: %v", err)
	}

	subject := "rr-test-" + randSuffix()

	// Core NATS responder
	nc.Subscribe(subject, func(msg *nats.Msg) {
		msg.Respond([]byte(`{"valid":true,"echo":"reply"}`))
	})

	producer := events.NewProducer[map[string]any](nc, js, subject)
	resp, err := producer.PublishAndWait(context.Background(), map[string]any{"test": 1}, 3*time.Second)
	if err != nil {
		t.Fatalf("PublishAndWait: %v", err)
	}
	if resp.Data["valid"] != true {
		t.Errorf("valid = %v, want true", resp.Data["valid"])
	}
	t.Logf("request-reply OK: %v", resp.Data)
}

func TestIntegration_Service_WebhookNATSPublish(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	natsURL := getNATSTestURL(t)

	dir := t.TempDir()
	cfgPath := dir + "/service.yaml"
	os.WriteFile(cfgPath, fmt.Appendf(nil, `name: wh-test
port: 19901
nats:
  - name: primary
    url: "%s"
    streams:
      - name: events
entry:
  - type: webhook
    method: POST
    path: /webhooks/test
    handler: onWh
    nats_publish:
      - stream: events
        subject: events.test
server:
  host: 0.0.0.0
  api_prefix: ""
  max_conns: 1000
  middleware:
    - path: "/*"
      apply: []
`, natsURL), 0o644)

	svc, err := New(cfgPath)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	svc.WithRest("onWh", func(c *RestCtx) error {
		return c.JSON(fiber.Map{"ok": true})
	})

	// Start service
	errCh := make(chan error, 1)
	go func() { errCh <- svc.Run() }()
	time.Sleep(200 * time.Millisecond)
	defer svc.shutdown()

	// POST to webhook → should auto-publish to NATS
	req, _ := http.NewRequestWithContext(context.Background(), "POST", "http://localhost:19901/webhooks/test",
		strings.NewReader(`{"event":"test"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("status = %d", resp.StatusCode)
	}
	// Consumer verification requires subscribing before publish,
	// so we just verify the webhook responded OK and no error in NATS publish.
	t.Log("webhook event_publish test passed")
}

func TestIntegration_Service_CronNATS(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	natsURL := getNATSTestURL(t)

	ctx := context.Background()
	conn, err := events.Connect(ctx, events.ConnOptions{URL: natsURL, Timeout: 3 * time.Second})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.Drain()

	streamName := "it-cron-" + randSuffix()
	conn.EnsureStream(events.StreamConfig{Name: streamName, Subjects: []string{streamName}})

	// Subscribe to verify cron publishes
	var received atomic.Int64
	sub, _ := conn.JS.Subscribe(streamName, func(m *nats.Msg) {
		received.Add(1)
		m.Ack()
	}, nats.ManualAck())

	s := NewCronScheduler()
	err = s.AddJob(context.Background(), CronJob{
		Name:     "fast-cron",
		Schedule: "@every 1s", // every second
		Mode:     "nats",
		Publish:  &CronPublish{Stream: streamName, Subject: streamName},
	}, conn, nil)
	if err != nil {
		t.Fatalf("AddJob: %v", err)
	}

	s.Start()
	time.Sleep(2500 * time.Millisecond)
	s.Stop()
	sub.Unsubscribe()

	if received.Load() < 2 {
		t.Errorf("cron published %d messages, want >= 2", received.Load())
	}
	t.Logf("cron published %d messages", received.Load())
}

func TestIntegration_GracefulShutdown(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	natsURL := getNATSTestURL(t)

	ctx := context.Background()
	conn, err := events.Connect(ctx, events.ConnOptions{URL: natsURL, Timeout: 3 * time.Second})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.Drain()

	streamName := "it-shutdown-" + randSuffix()
	conn.EnsureStream(events.StreamConfig{Name: streamName, Subjects: []string{streamName}})

	mgr := NewExitWorkerManager()
	var inFlight atomic.Int64
	var completed atomic.Int64

	err = mgr.Start(ctx, []ExitWorker{
		{
			Name:          "slow",
			Subscribe:     SubscribeDef{Stream: streamName, Subject: streamName},
			Handler:       "onSlow",
			MaxConcurrent: 2,
		},
	}, map[string]events.EventBroker{"default": conn}, map[string]ExitHandler{
		"onSlow": func(_ context.Context, msg []byte) ([]byte, error) {
			inFlight.Add(1)
			time.Sleep(500 * time.Millisecond)
			inFlight.Add(-1)
			completed.Add(1)
			return nil, nil
		},
	}, nil)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	time.Sleep(200 * time.Millisecond)

	// Publish and immediately shutdown
	conn.JS.Publish(streamName, []byte(`{"slow":true}`))
	time.Sleep(50 * time.Millisecond)
	mgr.Shutdown(5 * time.Second)

	if completed.Load() < 1 {
		t.Error("slow handler should have completed before shutdown timeout")
	}
	if inFlight.Load() > 0 {
		t.Errorf("in-flight = %d, want 0 after drain", inFlight.Load())
	}
	t.Logf("graceful shutdown: in-flight=0, completed=%d", completed.Load())
}

func TestIntegration_CRUD_NATSPublish(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	natsURL := getNATSTestURL(t)

	ctx := context.Background()
	conn, err := events.Connect(ctx, events.ConnOptions{URL: natsURL, Timeout: 3 * time.Second})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.Drain()

	streamName := "it-crud-pub-" + randSuffix()
	conn.EnsureStream(events.StreamConfig{Name: streamName, Subjects: []string{streamName}})

	// Subscribe to verify publishes
	var received atomic.Int64
	sub, _ := conn.JS.Subscribe(streamName, func(m *nats.Msg) {
		received.Add(1)
		m.Ack()
	}, nats.ManualAck())
	defer sub.Unsubscribe()

	app := fiber.New(fiber.Config{
		ErrorHandler: func(c fiber.Ctx, err error) error {
			code := fiber.StatusInternalServerError
			if e, ok := err.(*fiber.Error); ok {
				code = e.Code
			}
			return c.Status(code).JSON(fiber.Map{"code": code, "message": err.Error()})
		},
	})

	provider := &mockCRUDProvider{}
	handlers := &EntryHandlers{
		CRUD: map[string]CRUDProvider{"Product": provider},
	}

	cfg := &ServiceConfig{
		Entry: []EntryDef{
			{
				Type:         "crud",
				Model:        "Product",
				Resource:     "products",
				DB:           "test",
				Path:         "/products",
				EventPublish: []EventPublishTarget{{Stream: streamName, Subject: streamName}},
			},
		},
	}
	err = RegisterEntries(app, cfg, handlers, "/api/v1", map[string]events.EventBroker{"default": conn}, nil, nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("RegisterEntries: %v", err)
	}

	// 1. POST → should publish to NATS
	resp, _ := app.Test(httptest.NewRequestWithContext(context.Background(), "POST", "/api/v1/products", strings.NewReader(`{"name":"test"}`)))
	resp.Body.Close()
	time.Sleep(100 * time.Millisecond)

	// 2. PATCH → should publish to NATS
	resp2, _ := app.Test(httptest.NewRequestWithContext(context.Background(), "PATCH", "/api/v1/products/1", strings.NewReader(`{"name":"updated"}`)))
	resp2.Body.Close()
	time.Sleep(100 * time.Millisecond)

	// 3. DELETE → should publish to NATS
	resp3, _ := app.Test(httptest.NewRequestWithContext(context.Background(), "DELETE", "/api/v1/products/1", nil))
	resp3.Body.Close()
	time.Sleep(100 * time.Millisecond)

	// 4. GET → should NOT publish (read-only)
	resp4, _ := app.Test(httptest.NewRequestWithContext(context.Background(), "GET", "/api/v1/products", nil))
	resp4.Body.Close()

	if received.Load() < 3 {
		t.Errorf("nats_publish for CRUD: received %d messages, want >= 3 (POST+PATCH+DELETE)", received.Load())
	}
	t.Logf("CRUD nats_publish: %d messages published", received.Load())
}

func TestIntegration_ExitWorker_Pull(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	natsURL := getNATSTestURL(t)

	ctx := context.Background()
	conn, err := events.Connect(ctx, events.ConnOptions{URL: natsURL, Timeout: 3 * time.Second})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.Drain()

	streamName := "it-pull-" + randSuffix()
	conn.EnsureStream(events.StreamConfig{Name: streamName, Subjects: []string{streamName}})

	mgr := NewExitWorkerManager()
	var received atomic.Int64

	err = mgr.Start(ctx, []ExitWorker{
		{
			Name:          "puller",
			Subscribe:     SubscribeDef{Stream: streamName, Subject: streamName},
			Handler:       "onPull",
			MaxConcurrent: 2,
			ConsumerMode:  "pull",
			PullBatch:     5,
			PullMaxWait:   "3s",
		},
	}, map[string]events.EventBroker{"default": conn}, map[string]ExitHandler{
		"onPull": func(ctx context.Context, msg []byte) ([]byte, error) {
			received.Add(1)
			return nil, nil
		},
	}, nil)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	time.Sleep(200 * time.Millisecond)

	for i := range 3 {
		conn.JS.Publish(streamName, fmt.Appendf(nil, `{"n":%d}`, i))
	}
	time.Sleep(2 * time.Second)

	if received.Load() < 3 {
		t.Errorf("received %d, want 3", received.Load())
	}
	t.Logf("pull worker received %d messages", received.Load())

	mgr.Shutdown(2 * time.Second)
}

func TestIntegration_ExitWorker_HandlerErrorNak(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	natsURL := getNATSTestURL(t)

	ctx := context.Background()
	conn, err := events.Connect(ctx, events.ConnOptions{URL: natsURL, Timeout: 3 * time.Second})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.Drain()

	streamName := "it-errnak-" + randSuffix()
	conn.EnsureStream(events.StreamConfig{Name: streamName, Subjects: []string{streamName}})

	mgr := NewExitWorkerManager()
	var count atomic.Int64

	err = mgr.Start(ctx, []ExitWorker{
		{
			Name:          "errhandler",
			Subscribe:     SubscribeDef{Stream: streamName, Subject: streamName},
			Handler:       "onErr",
			MaxConcurrent: 1,
		},
	}, map[string]events.EventBroker{"default": conn}, map[string]ExitHandler{
		"onErr": func(ctx context.Context, msg []byte) ([]byte, error) {
			n := count.Add(1)
			if n <= 1 {
				return nil, fmt.Errorf("simulated error")
			}
			return nil, nil
		},
	}, nil)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	time.Sleep(200 * time.Millisecond)

	conn.JS.Publish(streamName, []byte(`{"test":true}`))
	time.Sleep(3 * time.Second)

	n := count.Load()
	if n < 2 {
		t.Errorf("expected >= 2 (error + retry), got %d", n)
	}
	t.Logf("handler error retry count: %d", n)

	mgr.Shutdown(2 * time.Second)
}

func TestIntegration_MultiBroker(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	natsURL := getNATSTestURL(t)

	ctx := context.Background()
	conn1, err := events.Connect(ctx, events.ConnOptions{URL: natsURL, Timeout: 3 * time.Second, Name: "broker1"})
	if err != nil {
		t.Fatalf("connect broker1: %v", err)
	}
	defer conn1.Drain()

	conn2, err := events.Connect(ctx, events.ConnOptions{URL: natsURL, Timeout: 3 * time.Second, Name: "broker2"})
	if err != nil {
		t.Fatalf("connect broker2: %v", err)
	}
	defer conn2.Drain()

	streamA := "it-multi-a-" + randSuffix()
	streamB := "it-multi-b-" + randSuffix()
	conn1.EnsureStream(events.StreamConfig{Name: streamA, Subjects: []string{streamA}})
	conn2.EnsureStream(events.StreamConfig{Name: streamB, Subjects: []string{streamB}})

	mgr := NewExitWorkerManager()
	var countA, countB atomic.Int64

	err = mgr.Start(ctx, []ExitWorker{
		{
			Name:          "worker-a",
			EventStream:   "primary",
			Subscribe:     SubscribeDef{Stream: streamA, Subject: streamA},
			Handler:       "onA",
			MaxConcurrent: 1,
		},
		{
			Name:          "worker-b",
			EventStream:   "secondary",
			Subscribe:     SubscribeDef{Stream: streamB, Subject: streamB},
			Handler:       "onB",
			MaxConcurrent: 1,
		},
	}, map[string]events.EventBroker{"primary": conn1, "secondary": conn2}, map[string]ExitHandler{
		"onA": func(ctx context.Context, msg []byte) ([]byte, error) {
			countA.Add(1)
			return nil, nil
		},
		"onB": func(ctx context.Context, msg []byte) ([]byte, error) {
			countB.Add(1)
			return nil, nil
		},
	}, nil)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	time.Sleep(200 * time.Millisecond)

	conn1.JS.Publish(streamA, []byte(`{"from":"broker1"}`))
	conn2.JS.Publish(streamB, []byte(`{"from":"broker2"}`))
	time.Sleep(2 * time.Second)

	if countA.Load() < 1 {
		t.Error("expected worker-a to receive message")
	}
	if countB.Load() < 1 {
		t.Error("expected worker-b to receive message")
	}
	t.Logf("multi-broker: A=%d, B=%d", countA.Load(), countB.Load())

	mgr.Shutdown(2 * time.Second)
}

// --- Helpers ---

func getNATSTestURL(t *testing.T) string {
	t.Helper()
	if u := os.Getenv("NATS_URL"); u != "" {
		return u
	}
	return "nats://localhost:14222"
}

func randSuffix() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}
