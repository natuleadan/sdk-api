package events

import (
	"context"
	"fmt"
	"time"

	"github.com/goccy/go-json"
	"github.com/nats-io/nats.go"
)

type AckAction int

const (
	Ack    AckAction = iota
	Nak
	NakDelay
	Term
)

type Msg[T any] struct {
	Data T
	Raw  *nats.Msg
	Ack  func(opts ...nats.AckOpt) error
	Nak  func(opts ...nats.AckOpt) error
	NakD func(delay time.Duration, opts ...nats.AckOpt) error
	Term func(opts ...nats.AckOpt) error
}

type Handler[T any] func(ctx context.Context, msg Msg[T]) (AckAction, error)

type ConsumerConfig struct {
	Stream       string
	Subject      string
	Durable      string
	QueueGroup   string
	DeliverAll   bool
	MaxDeliver   int
	AckWait      time.Duration
	BackOff      []time.Duration
	PullBatch    int
	PullMaxWait  time.Duration
	NakDelay     time.Duration
	Reply        bool // if true, handler returns data for msg.Respond()
}

func DefaultConsumerConfig(stream, durable string) ConsumerConfig {
	return ConsumerConfig{
		Stream:       stream,
		Subject:      stream,
		Durable:      durable,
		DeliverAll:   true,
		MaxDeliver:   5,
		AckWait:      30 * time.Second,
		BackOff:      []time.Duration{1 * time.Second, 2 * time.Second, 5 * time.Second, 10 * time.Second, 30 * time.Second},
		PullBatch:    10,
		PullMaxWait:  5 * time.Second,
		NakDelay:     5 * time.Second,
	}
}

func getNakDelay(cfg ConsumerConfig) time.Duration {
	if cfg.NakDelay > 0 {
		return cfg.NakDelay
	}
	return 5 * time.Second
}

func consumerSubOpts(cfg ConsumerConfig) []nats.SubOpt {
	opts := []nats.SubOpt{
		nats.MaxDeliver(cfg.MaxDeliver),
		nats.AckWait(cfg.AckWait),
	}
	if cfg.DeliverAll {
		opts = append(opts, nats.DeliverAll())
	} else {
		opts = append(opts, nats.DeliverNew())
	}
	if len(cfg.BackOff) > 0 {
		opts = append(opts, nats.BackOff(cfg.BackOff))
	}
	return opts
}

func ConsumePull[T any](ctx context.Context, js nats.JetStreamContext, cfg ConsumerConfig, handler Handler[T]) error {
	_ = js.DeleteConsumer(cfg.Stream, cfg.Durable)
	opts := consumerSubOpts(cfg)

	sub, err := js.PullSubscribe(cfg.Subject, cfg.Durable, opts...)
	if err != nil {
		return fmt.Errorf("events: pull subscribe: %w", err)
	}

	pullBatch := cfg.PullBatch
	if pullBatch <= 0 { pullBatch = 10 }
	pullMaxWait := cfg.PullMaxWait
	if pullMaxWait <= 0 { pullMaxWait = 5 * time.Second }

	go func() {
		defer func() { _ = sub.Unsubscribe() }()
		for {
			msgs, err := sub.Fetch(pullBatch, nats.MaxWait(pullMaxWait))
			if err != nil {
				if err == nats.ErrTimeout {
					continue
				}
				select {
				case <-ctx.Done():
					return
				default:
					continue
				}
			}
			for _, m := range msgs {
				select {
				case <-ctx.Done():
					return
				default:
				}
				processMsg(ctx, m, handler, cfg)
			}
		}
	}()

	return nil
}

func ConsumePush[T any](ctx context.Context, js nats.JetStreamContext, cfg ConsumerConfig, handler Handler[T]) error {
	_ = js.DeleteConsumer(cfg.Stream, cfg.Durable)

	opts := append([]nats.SubOpt{
		nats.Durable(cfg.Durable),
		nats.ManualAck(),
	}, consumerSubOpts(cfg)...)

	var (
		sub *nats.Subscription
		err error
	)

	fn := func(m *nats.Msg) {
		processMsg(ctx, m, handler, cfg)
	}

	if cfg.QueueGroup != "" {
		sub, err = js.QueueSubscribe(cfg.Subject, cfg.QueueGroup, fn, opts...)
	} else {
		sub, err = js.Subscribe(cfg.Subject, fn, opts...)
	}
	if err != nil {
		return fmt.Errorf("events: push subscribe: %w", err)
	}

	go func() {
		<-ctx.Done()
		_ = sub.Unsubscribe()
	}()

	return nil
}

func processMsg[T any](ctx context.Context, m *nats.Msg, handler Handler[T], cfg ConsumerConfig) {
	var data T
	if err := json.Unmarshal(m.Data, &data); err != nil {
		_ = m.Term()
		return
	}

	msg := Msg[T]{
		Data: data,
		Raw:  m,
		Ack:  m.Ack,
		Nak:  m.Nak,
		NakD: m.NakWithDelay,
		Term: m.Term,
	}

	action, err := handler(ctx, msg)
	if err != nil {
		_ = m.Nak()
		return
	}

	switch action {
	case Ack:
		_ = m.Ack()
	case Nak:
		_ = m.Nak()
	case NakDelay:
		_ = m.NakWithDelay(getNakDelay(cfg))
	case Term:
		_ = m.Term()
	default:
		_ = m.Ack()
	}
}
