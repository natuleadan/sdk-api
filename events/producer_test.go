package events

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
)

func TestProducer_PublishAndWait(t *testing.T) {
	natsURL := os.Getenv("NATS_URL")
	if natsURL == "" {
		t.Skip("NATS_URL not set")
	}

	nc, err := nats.Connect(natsURL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer nc.Drain()

	js, err := nc.JetStream()
	if err != nil {
		t.Fatalf("jetstream: %v", err)
	}

	subject := "req-reply-test-" + time.Now().Format("150405")

	// Subscribe a responder
	_, err = nc.Subscribe(subject, func(msg *nats.Msg) {
		msg.Respond([]byte(`{"ok":true,"echo":"reply"}`))
	})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	// Create producer for request-reply
	producer := NewProducer[map[string]any](nc, js, subject)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := producer.PublishAndWait(ctx, map[string]any{"action": "test"}, 3*time.Second)
	if err != nil {
		t.Fatalf("PublishAndWait: %v", err)
	}

	if resp.Data["ok"] != true {
		t.Errorf("response: %v", resp.Data)
	}
	if resp.Data["echo"] != "reply" {
		t.Errorf("echo = %v", resp.Data["echo"])
	}

	// Test raw version as well
	respRaw, err := producer.PublishAndWaitRaw(ctx, []byte(`{"raw":true}`), 2*time.Second)
	if err != nil {
		t.Fatalf("PublishAndWaitRaw: %v", err)
	}
	if respRaw.Subject != _INBOX_PREFIX {
		// Response comes on inbox, subject is the inbox prefix
		t.Logf("raw response subject: %s", respRaw.Subject)
	}
	if respRaw.Data == nil || len(respRaw.Data) == 0 {
		t.Error("empty raw response")
	}
}

func TestProducer_PublishAndWait_Timeout(t *testing.T) {
	natsURL := os.Getenv("NATS_URL")
	if natsURL == "" {
		t.Skip("NATS_URL not set")
	}

	nc, err := nats.Connect(natsURL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer nc.Drain()

	js, err := nc.JetStream()
	if err != nil {
		t.Fatalf("jetstream: %v", err)
	}

	subject := "req-reply-timeout-" + time.Now().Format("150405")
	producer := NewProducer[map[string]any](nc, js, subject)

	ctx := context.Background()
	// No responder → should timeout
	_, err = producer.PublishAndWait(ctx, map[string]any{}, 500*time.Millisecond)
	if err == nil {
		t.Error("expected timeout error")
	}
	t.Logf("timeout error (expected): %v", err)
}

// _INBOX_PREFIX is used in test to check response subject
const _INBOX_PREFIX = "_INBOX."
