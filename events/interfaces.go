package events

import (
	"context"
	"time"
)

type EventBroker interface {
	Name() string
	Publish(ctx context.Context, subject string, data []byte) error
	Subscribe(ctx context.Context, subject string, durable string, handler MessageHandler) (Subscription, error)
	PullSubscribe(ctx context.Context, subject string, durable string) (PullConsumer, error)
	Request(ctx context.Context, subject string, data []byte, timeout time.Duration) ([]byte, error)
	EnsureStream(cfg StreamConfig) error
	EnsureStreams(configs ...StreamConfig) error
	Close() error
}

type MessageHandler func(ctx context.Context, msg Message) error

type Message interface {
	Data() []byte
	Subject() string
	Ack() error
	Nak(delay ...time.Duration) error
	Term() error
	Respond(data []byte) error
}

type Subscription interface {
	Unsubscribe() error
}

type PullConsumer interface {
	Fetch(batch int, maxWait time.Duration) ([]Message, error)
	Unsubscribe() error
}

type StorageType int

const (
	FileStorage   StorageType = iota
	MemoryStorage
)

type CompressionType int

const (
	S2Compression  CompressionType = iota
	NoCompression
)

type StreamConfig struct {
	Name        string
	Subjects    []string
	MaxAge      time.Duration
	MaxBytes    int64
	Storage     StorageType
	Compression CompressionType
}

func DefaultStreamConfig(name string) StreamConfig {
	return StreamConfig{
		Name:        name,
		Subjects:    []string{name, name + ".>"},
		MaxAge:      24 * time.Hour,
		MaxBytes:    1 * 1024 * 1024 * 1024,
		Storage:     FileStorage,
		Compression: S2Compression,
	}
}
