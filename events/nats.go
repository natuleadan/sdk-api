package events

import (
	"context"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
)

type StreamConfig struct {
	Name        string
	Subjects    []string
	MaxAge      time.Duration
	MaxBytes    int64
	Storage     nats.StorageType
	Compression nats.StoreCompression
}

func DefaultStreamConfig(name string) StreamConfig {
	return StreamConfig{
		Name:        name,
		Subjects:    []string{name, name + ".>"},
		MaxAge:      24 * time.Hour,
		MaxBytes:    1 * 1024 * 1024 * 1024,
		Storage:     nats.FileStorage,
		Compression: nats.S2Compression,
	}
}

type ConnOptions struct {
	URL             string
	MaxReconnects   int
	ReconnectWait   time.Duration
	Timeout         time.Duration
	RetryOnFail     bool
}

type Conn struct {
	NC  *nats.Conn
	JS  nats.JetStreamContext
	ctx context.Context
	kvs map[string]nats.KeyValue
}

func Connect(ctx context.Context, opts ConnOptions) (*Conn, error) {
	url := opts.URL
	if url == "" {
		url = nats.DefaultURL
	}
	maxReconnects := opts.MaxReconnects
	if maxReconnects <= 0 {
		maxReconnects = 10
	}
	reconnectWait := opts.ReconnectWait
	if reconnectWait <= 0 {
		reconnectWait = 2 * time.Second
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	nc, err := nats.Connect(url,
		nats.RetryOnFailedConnect(opts.RetryOnFail),
		nats.MaxReconnects(maxReconnects),
		nats.ReconnectWait(reconnectWait),
		nats.Timeout(timeout),
	)
	if err != nil {
		return nil, fmt.Errorf("events: connect: %w", err)
	}

	js, err := nc.JetStream()
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("events: jetstream: %w", err)
	}

	return &Conn{NC: nc, JS: js, ctx: ctx, kvs: make(map[string]nats.KeyValue)}, nil
}

func (c *Conn) EnsureStream(cfg StreamConfig) error {
	_, err := c.JS.AddStream(&nats.StreamConfig{
		Name:        cfg.Name,
		Subjects:    cfg.Subjects,
		Storage:     cfg.Storage,
		MaxAge:      cfg.MaxAge,
		MaxBytes:    cfg.MaxBytes,
		Compression: cfg.Compression,
		AllowMsgTTL: true,
	})
	if err != nil && err != nats.ErrStreamNameAlreadyInUse {
		return fmt.Errorf("events: stream %s: %w", cfg.Name, err)
	}
	return nil
}

func (c *Conn) EnsureStreams(configs ...StreamConfig) error {
	for _, cfg := range configs {
		if err := c.EnsureStream(cfg); err != nil {
			return err
		}
	}
	return nil
}

type KVConfig struct {
	Bucket      string
	Description string
	TTL         time.Duration
	MaxBytes    int64
	Storage     nats.StorageType
}

func DefaultKVConfig(bucket string) KVConfig {
	return KVConfig{
		Bucket:      bucket,
		Description: bucket + " KV store",
		TTL:         5 * time.Minute,
		MaxBytes:    256 * 1024 * 1024,
		Storage:     nats.MemoryStorage,
	}
}

func (c *Conn) EnsureKeyValue(cfg KVConfig) (nats.KeyValue, error) {
	if kv, ok := c.kvs[cfg.Bucket]; ok {
		return kv, nil
	}
	kv, err := c.JS.CreateKeyValue(&nats.KeyValueConfig{
		Bucket:      cfg.Bucket,
		Description: cfg.Description,
		TTL:         cfg.TTL,
		MaxBytes:    cfg.MaxBytes,
		Storage:     cfg.Storage,
	})
	if err != nil && err != nats.ErrStreamNameAlreadyInUse {
		return nil, fmt.Errorf("events: kv %s: %w", cfg.Bucket, err)
	}
	if err == nats.ErrStreamNameAlreadyInUse {
		kv, err = c.JS.KeyValue(cfg.Bucket)
		if err != nil {
			return nil, fmt.Errorf("events: kv load %s: %w", cfg.Bucket, err)
		}
	}
	c.kvs[cfg.Bucket] = kv
	return kv, nil
}

func (c *Conn) Drain() {
	if c.NC == nil {
		return
	}
	c.NC.Drain()
	c.NC.Close()
}

func (c *Conn) Context() context.Context {
	return c.ctx
}

func WaitForNATS(ctx context.Context, url string, retries int, delay time.Duration) error {
	for i := range retries {
		nc, err := nats.Connect(url, nats.Timeout(3*time.Second))
		if err == nil {
			nc.Close()
			return nil
		}
		if i < retries-1 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
		}
	}
	return fmt.Errorf("events: NATS not reachable after %d retries", retries)
}
