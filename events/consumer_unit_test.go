package events

import (
	"testing"
	"time"

	"github.com/nats-io/nats.go"
)

func TestDefaultConsumerConfig(t *testing.T) {
	cfg := DefaultConsumerConfig("orders", "ord-consumer")
	if cfg.Stream != "orders" {
		t.Errorf("Stream = %q", cfg.Stream)
	}
	if cfg.Subject != "orders" {
		t.Errorf("Subject = %q", cfg.Subject)
	}
	if cfg.Durable != "ord-consumer" {
		t.Errorf("Durable = %q", cfg.Durable)
	}
	if !cfg.DeliverAll {
		t.Error("DeliverAll should be true")
	}
	if cfg.MaxDeliver != 5 {
		t.Errorf("MaxDeliver = %d", cfg.MaxDeliver)
	}
	if cfg.AckWait != 30*time.Second {
		t.Errorf("AckWait = %v", cfg.AckWait)
	}
	if cfg.PullBatch != 10 {
		t.Errorf("PullBatch = %d", cfg.PullBatch)
	}
	if cfg.PullMaxWait != 5*time.Second {
		t.Errorf("PullMaxWait = %v", cfg.PullMaxWait)
	}
	if cfg.NakDelay != 5*time.Second {
		t.Errorf("NakDelay = %v", cfg.NakDelay)
	}
}

func TestConsumerConfig_ReplyField(t *testing.T) {
	cfg := ConsumerConfig{
		Stream:  "s",
		Durable: "d",
		Reply:   true,
	}
	if !cfg.Reply {
		t.Error("Reply field should be settable to true")
	}

	cfg2 := ConsumerConfig{Stream: "s", Durable: "d", Reply: false}
	if cfg2.Reply {
		t.Error("Reply field should be settable to false")
	}
}

func TestAckAction_Values(t *testing.T) {
	if Ack != 0 {
		t.Errorf("Ack = %d", Ack)
	}
	if Nak != 1 {
		t.Errorf("Nak = %d", Nak)
	}
	if NakDelay != 2 {
		t.Errorf("NakDelay = %d", NakDelay)
	}
	if Term != 3 {
		t.Errorf("Term = %d", Term)
	}
}

func TestGetNakDelay(t *testing.T) {
	cfg := ConsumerConfig{NakDelay: 10 * time.Second}
	if getNakDelay(cfg) != 10*time.Second {
		t.Errorf("getNakDelay = %v", getNakDelay(cfg))
	}

	cfg2 := ConsumerConfig{NakDelay: 0}
	if getNakDelay(cfg2) != 5*time.Second {
		t.Errorf("getNakDelay default = %v", getNakDelay(cfg2))
	}
}

func TestMsgStruct(t *testing.T) {
	msg := Msg[string]{Data: "hello"}
	if msg.Data != "hello" {
		t.Errorf("Data = %q", msg.Data)
	}
	if msg.Raw != nil {
		t.Error("Raw should be nil")
	}
}

func TestConsumerSubOpts(t *testing.T) {
	cfg := ConsumerConfig{
		MaxDeliver: 3,
		AckWait:    10 * time.Second,
		DeliverAll: true,
		BackOff:    []time.Duration{1 * time.Second, 5 * time.Second},
	}
	opts := consumerSubOpts(cfg)
	if len(opts) != 4 {
		t.Errorf("expected 4 opts, got %d", len(opts))
	}

	cfg.DeliverAll = false
	opts = consumerSubOpts(cfg)
	if len(opts) != 4 {
		t.Errorf("expected 4 opts (DeliverNew counted), got %d", len(opts))
	}
}

func TestConvertStorage(t *testing.T) {
	if got := convertStorage(FileStorage); got != nats.FileStorage {
		t.Errorf("FileStorage = %v, want %v", got, nats.FileStorage)
	}
	if got := convertStorage(MemoryStorage); got != nats.MemoryStorage {
		t.Errorf("MemoryStorage = %v, want %v", got, nats.MemoryStorage)
	}
	var unknown StorageType = 99
	if got := convertStorage(unknown); got != nats.FileStorage {
		t.Errorf("unknown StorageType = %v, want FileStorage", got)
	}
}

func TestConvertCompression(t *testing.T) {
	if got := convertCompression(S2Compression); got != nats.S2Compression {
		t.Errorf("S2Compression = %v, want %v", got, nats.S2Compression)
	}
	if got := convertCompression(NoCompression); got != nats.NoCompression {
		t.Errorf("NoCompression = %v, want %v", got, nats.NoCompression)
	}
	var unknown CompressionType = 99
	if got := convertCompression(unknown); got != nats.S2Compression {
		t.Errorf("unknown CompressionType = %v, want S2Compression", got)
	}
}

func TestDefaultKVConfig(t *testing.T) {
	cfg := DefaultKVConfig("test-bucket")
	if cfg.Bucket != "test-bucket" {
		t.Errorf("Bucket = %q", cfg.Bucket)
	}
	if cfg.Description != "test-bucket KV store" {
		t.Errorf("Description = %q", cfg.Description)
	}
	if cfg.TTL != 5*time.Minute {
		t.Errorf("TTL = %v", cfg.TTL)
	}
	if cfg.MaxBytes != 256*1024*1024 {
		t.Errorf("MaxBytes = %d", cfg.MaxBytes)
	}
	if cfg.Storage != MemoryStorage {
		t.Errorf("Storage = %v", cfg.Storage)
	}
}
