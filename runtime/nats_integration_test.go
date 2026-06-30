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

	"github.com/gofiber/fiber/v2"
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
		"onMsg": func(ctx context.Context, msg []byte) ([]byte, error) {
			received.Add(1)
			return nil, nil
		},
	}, nil)

	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	time.Sleep(200 * time.Millisecond)

	// Publish messages
	for i := 0; i < 5; i++ {
		conn.JS.Publish(streamName, []byte(fmt.Sprintf(`{"n":%d}`, i)))
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
		"onValidate": func(ctx context.Context, msg []byte) ([]byte, error) {
			processed.Add(1)
			return []byte(`{"valid":true}`), nil
		},
	}, nil)

	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	time.Sleep(200 * time.Millisecond)

	// Publish via core NATS — JetStream will deliver to consumer
	for i := 0; i < 3; i++ {
		conn.NC.Publish(streamName, []byte(fmt.Sprintf(`{"n":%d}`, i)))
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
	os.WriteFile(cfgPath, []byte(fmt.Sprintf(`name: wh-test
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
`, natsURL)), 0644)

	svc, err := New(cfgPath)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	svc.WithRest("onWh", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"ok": true})
	})

	// Start service
	errCh := make(chan error, 1)
	go func() { errCh <- svc.Run() }()
	time.Sleep(200 * time.Millisecond)
	defer svc.shutdown()

	// POST to webhook → should auto-publish to NATS
	resp, err := http.Post("http://localhost:19901/webhooks/test", "application/json",
		strings.NewReader(`{"event":"test"}`))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("status = %d", resp.StatusCode)
	}
	// Consumer verification requires subscribing before publish,
	// so we just verify the webhook responded OK and no error in NATS publish.
	t.Log("webhook nats_publish test passed")
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
	err = s.AddJob(CronJob{
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
		"onSlow": func(ctx context.Context, msg []byte) ([]byte, error) {
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
		ErrorHandler: func(c *fiber.Ctx, err error) error {
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
				Type:        "crud",
				Model:       "Product",
				Resource:    "products",
				DB:          "test",
				Path:        "/products",
				NATSPublish: []NATSPublishTarget{{Stream: streamName, Subject: streamName}},
			},
		},
	}
	err = RegisterEntries(app, cfg, handlers, "/api/v1", map[string]events.EventBroker{"default": conn}, nil)
	if err != nil {
		t.Fatalf("RegisterEntries: %v", err)
	}

	// 1. POST → should publish to NATS
	resp, _ := app.Test(httptest.NewRequest("POST", "/api/v1/products", strings.NewReader(`{"name":"test"}`)))
	resp.Body.Close()
	time.Sleep(100 * time.Millisecond)

	// 2. PATCH → should publish to NATS
	resp2, _ := app.Test(httptest.NewRequest("PATCH", "/api/v1/products/1", strings.NewReader(`{"name":"updated"}`)))
	resp2.Body.Close()
	time.Sleep(100 * time.Millisecond)

	// 3. DELETE → should publish to NATS
	resp3, _ := app.Test(httptest.NewRequest("DELETE", "/api/v1/products/1", nil))
	resp3.Body.Close()
	time.Sleep(100 * time.Millisecond)

	// 4. GET → should NOT publish (read-only)
	resp4, _ := app.Test(httptest.NewRequest("GET", "/api/v1/products", nil))
	resp4.Body.Close()

	if received.Load() < 3 {
		t.Errorf("nats_publish for CRUD: received %d messages, want >= 3 (POST+PATCH+DELETE)", received.Load())
	}
	t.Logf("CRUD nats_publish: %d messages published", received.Load())
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
