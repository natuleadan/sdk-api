//go:build integration

package events

import (
	"context"
	"fmt"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

func TestKafkaBroker_Name(t *testing.T) {
	b := NewKafkaBroker("test", []string{"localhost:9092"}, "test-group")
	if b.Name() != "test" {
		t.Errorf("Name = %q, want test", b.Name())
	}
	b.Close()
}

func TestKafkaBroker_PullSubscribeNotSupported(t *testing.T) {
	b := NewKafkaBroker("test", []string{"localhost:9092"}, "test-group")
	defer b.Close()

	_, err := b.PullSubscribe(context.Background(), "test", "test")
	if err == nil {
		t.Error("expected error for pull subscribe")
	}
}

func TestKafkaBroker_RequestNotSupported(t *testing.T) {
	b := NewKafkaBroker("test", []string{"localhost:9092"}, "test-group")
	defer b.Close()

	_, err := b.Request(context.Background(), "test", nil, time.Second)
	if err == nil {
		t.Error("expected error for request")
	}
}

func TestKafkaBroker_EnsureStreamNoop(t *testing.T) {
	kafkaURL := os.Getenv("KAFKA_URL")
	if kafkaURL == "" {
		t.Skip("KAFKA_URL not set, skipping kafka test")
	}
	b := NewKafkaBroker("test", []string{kafkaURL}, "test-group")
	defer b.Close()

	if err := b.EnsureStream(StreamConfig{Name: "x"}); err != nil {
		t.Errorf("EnsureStream: %v", err)
	}
}

func TestIntegration_KafkaBroker_PublishConsume(t *testing.T) {
	kafkaURL := os.Getenv("KAFKA_URL")
	if kafkaURL == "" {
		t.Skip("KAFKA_URL not set, skipping integration test")
	}

	b := NewKafkaBroker("kafka-test", []string{kafkaURL}, "kafka-test-group")
	defer b.Close()

	topic := "it-kafka-broker-" + fmt.Sprintf("%d", time.Now().UnixNano())

	// Subscribe first
	var received atomic.Int64
	var lastMsg []byte
	done := make(chan struct{})

	sub, err := b.Subscribe(context.Background(), topic, "it-"+topic, func(_ context.Context, msg Message) error {
		received.Add(1)
		lastMsg = msg.Data()
		close(done)
		return nil
	})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer sub.Unsubscribe()

	// Wait for consumer group to be ready
	time.Sleep(2 * time.Second)

	// Publish
	err = b.Publish(context.Background(), topic, []byte(`{"hello":"kafka"}`))
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}

	select {
	case <-done:
		if string(lastMsg) != `{"hello":"kafka"}` {
			t.Errorf("got %q, want %q", string(lastMsg), `{"hello":"kafka"}`)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("timeout waiting for kafka message")
	}

	if received.Load() != 1 {
		t.Errorf("received %d messages, want 1", received.Load())
	}
}

func TestIntegration_KafkaBroker_MultipleMessages(t *testing.T) {
	kafkaURL := os.Getenv("KAFKA_URL")
	if kafkaURL == "" {
		t.Skip("KAFKA_URL not set, skipping integration test")
	}

	b := NewKafkaBroker("kafka-test", []string{kafkaURL}, "kafka-multi-group")
	defer b.Close()

	topic := "it-kafka-multi-" + fmt.Sprintf("%d", time.Now().UnixNano())

	var received atomic.Int64
	done := make(chan struct{})

	sub, err := b.Subscribe(context.Background(), topic, "it-"+topic, func(_ context.Context, msg Message) error {
		received.Add(1)
		if received.Load() >= 3 {
			close(done)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer sub.Unsubscribe()

	time.Sleep(2 * time.Second)

	for i := range 3 {
		err := b.Publish(context.Background(), topic, []byte(`{"n":`+fmt.Sprintf("%d", i)+`}`))
		if err != nil {
			t.Fatalf("Publish %d: %v", i, err)
		}
	}

	select {
	case <-done:
		if received.Load() != 3 {
			t.Errorf("received %d, want 3", received.Load())
		}
	case <-time.After(15 * time.Second):
		t.Fatal("timeout waiting for messages")
	}
}
