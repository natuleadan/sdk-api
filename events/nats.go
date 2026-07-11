package events

import (
	"context"
	"fmt"
	"time"

	"github.com/goccy/go-json"
	"github.com/nats-io/nats.go"
	"github.com/natuleadan/sdk-api/infra/logx"
)

type ConnOptions struct {
	Name            string
	URL             string
	MaxReconnects   int
	ReconnectWait   time.Duration
	Timeout         time.Duration
	RetryOnFail     bool
}

type Conn struct {
	name string
	NC   *nats.Conn
	JS   nats.JetStreamContext
	ctx  context.Context
	kvs  map[string]nats.KeyValue
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

	return &Conn{NC: nc, JS: js, ctx: ctx, kvs: make(map[string]nats.KeyValue), name: opts.Name}, nil
}

func (c *Conn) Name() string { return c.name }

func (c *Conn) Publish(ctx context.Context, subject string, data []byte) error {
	_, err := c.JS.Publish(subject, data)
	return err
}

func (c *Conn) PublishJSON(ctx context.Context, subject string, msg any) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("events: marshal: %w", err)
	}
	return c.Publish(ctx, subject, data)
}

func (c *Conn) Subscribe(ctx context.Context, subject string, durable string, handler MessageHandler) (Subscription, error) {
	sub, err := c.JS.Subscribe(subject, func(m *nats.Msg) {
		if err := handler(ctx, &natsMessage{msg: m}); err != nil {
		logx.Errorf("nats: handler error: %v", err)
	}
	}, nats.Durable(durable), nats.ManualAck(), nats.MaxDeliver(5), nats.AckWait(30*time.Second), nats.DeliverAll())
	if err != nil {
		return nil, err
	}
	return &natsSubscription{sub: sub}, nil
}

func (c *Conn) PullSubscribe(ctx context.Context, subject string, durable string) (PullConsumer, error) {
	sub, err := c.JS.PullSubscribe(subject, durable, nats.ManualAck(), nats.MaxDeliver(5), nats.AckWait(30*time.Second), nats.DeliverAll())
	if err != nil {
		return nil, err
	}
	return &natsPullConsumer{sub: sub}, nil
}

func (c *Conn) Request(ctx context.Context, subject string, data []byte, timeout time.Duration) ([]byte, error) {
	msg, err := c.NC.RequestWithContext(ctx, subject, data)
	if err != nil {
		return nil, err
	}
	return msg.Data, nil
}

func (c *Conn) EnsureStream(cfg StreamConfig) error {
	_, err := c.JS.AddStream(&nats.StreamConfig{
		Name:        cfg.Name,
		Subjects:    cfg.Subjects,
		Storage:     convertStorage(cfg.Storage),
		MaxAge:      cfg.MaxAge,
		MaxBytes:    cfg.MaxBytes,
		Compression: convertCompression(cfg.Compression),
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

func convertStorage(s StorageType) nats.StorageType {
	switch s {
	case MemoryStorage:
		return nats.MemoryStorage
	default:
		return nats.FileStorage
	}
}

func convertCompression(c CompressionType) nats.StoreCompression {
	switch c {
	case NoCompression:
		return nats.NoCompression
	default:
		return nats.S2Compression
	}
}

type KVConfig struct {
	Bucket      string
	Description string
	TTL         time.Duration
	MaxBytes    int64
	Storage     StorageType
}

func DefaultKVConfig(bucket string) KVConfig {
	return KVConfig{
		Bucket:      bucket,
		Description: bucket + " KV store",
		TTL:         5 * time.Minute,
		MaxBytes:    256 * 1024 * 1024,
		Storage:     MemoryStorage,
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
		Storage:     convertStorage(cfg.Storage),
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

func (c *Conn) KVGet(bucket, key string) ([]byte, error) {
	kv, err := c.EnsureKeyValue(DefaultKVConfig(bucket))
	if err != nil {
		return nil, err
	}
	entry, err := kv.Get(key)
	if err != nil {
		return nil, err
	}
	return entry.Value(), nil
}

func (c *Conn) KVPut(bucket, key string, value []byte) (uint64, error) {
	kv, err := c.EnsureKeyValue(DefaultKVConfig(bucket))
	if err != nil {
		return 0, err
	}
	rev, err := kv.Put(key, value)
	if err != nil {
		return 0, err
	}
	return rev, nil
}

func (c *Conn) KVDelete(bucket, key string) error {
	kv, err := c.EnsureKeyValue(DefaultKVConfig(bucket))
	if err != nil {
		return err
	}
	return kv.Delete(key)
}

func (c *Conn) SubscribeRaw(subject string, handler func(msg []byte)) error {
	_, err := c.NC.Subscribe(subject, func(m *nats.Msg) {
		handler(m.Data)
	})
	if err != nil {
		return err
	}
	if err := c.NC.Flush(); err != nil {
		logx.Errorf("nats: flush error: %v", err)
	}
	return nil
}

func (c *Conn) SubscribeRawReply(subject string, handler func(msg []byte) []byte) error {
	_, err := c.NC.Subscribe(subject, func(m *nats.Msg) {
		reply := handler(m.Data)
		if reply != nil {
			if err := m.Respond(reply); err != nil {
				logx.Errorf("nats: respond error: %v", err)
			}
		}
	})
	if err != nil {
		return err
	}
	if err := c.NC.Flush(); err != nil {
		logx.Errorf("nats: flush error: %v", err)
	}
	return nil
}

func (c *Conn) Drain() {
	if c.NC == nil {
		return
	}
	if err := c.NC.Drain(); err != nil {
		logx.Errorf("nats: drain error: %v", err)
	}
	c.NC.Close()
}

func (c *Conn) Close() error {
	if c.NC != nil {
		if err := c.NC.Drain(); err != nil {
		logx.Errorf("nats: drain error: %v", err)
	}
		c.NC.Close()
	}
	return nil
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

type natsMessage struct {
	msg *nats.Msg
}

func (m *natsMessage) Data() []byte                     { return m.msg.Data }
func (m *natsMessage) Subject() string                  { return m.msg.Subject }
func (m *natsMessage) Ack() error                       { return m.msg.Ack() }
func (m *natsMessage) Nak(delay ...time.Duration) error {
	if len(delay) > 0 {
		return m.msg.NakWithDelay(delay[0])
	}
	return m.msg.Nak()
}
func (m *natsMessage) Term() error                      { return m.msg.Term() }
func (m *natsMessage) Respond(data []byte) error        { return m.msg.Respond(data) }

type natsSubscription struct {
	sub *nats.Subscription
}

func (s *natsSubscription) Unsubscribe() error { return s.sub.Unsubscribe() }

type natsPullConsumer struct {
	sub *nats.Subscription
}

func (c *natsPullConsumer) Fetch(batch int, maxWait time.Duration) ([]Message, error) {
	msgs, err := c.sub.Fetch(batch, nats.MaxWait(maxWait))
	if err != nil {
		return nil, err
	}
	result := make([]Message, len(msgs))
	for i, m := range msgs {
		result[i] = &natsMessage{msg: m}
	}
	return result, nil
}

func (c *natsPullConsumer) Unsubscribe() error { return c.sub.Unsubscribe() }
