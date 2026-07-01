package events

import (
	"context"
	"fmt"
	"time"

	"github.com/goccy/go-json"
	"github.com/nats-io/nats.go"
	"github.com/natuleadan/sdk-api/infra/logx"
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
	if err := js.DeleteConsumer(cfg.Stream, cfg.Durable); err != nil {
		logx.Errorf("events: delete consumer error: %v", err)
	}
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
		defer func() {
		if err := sub.Unsubscribe(); err != nil {
			logx.Errorf("events: unsubscribe error: %v", err)
		}
	}()
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
	if err := js.DeleteConsumer(cfg.Stream, cfg.Durable); err != nil {
		logx.Errorf("events: delete consumer error: %v", err)
	}

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
		if err := sub.Unsubscribe(); err != nil {
			logx.Errorf("events: unsubscribe error: %v", err)
		}
	}()

	return nil
}

func processMsg[T any](ctx context.Context, m *nats.Msg, handler Handler[T], cfg ConsumerConfig) {
	var data T
	if err := json.Unmarshal(m.Data, &data); err != nil {
		if err := m.Term(); err != nil {
		logx.Errorf("events: term error: %v", err)
	}
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
		if err := m.Nak(); err != nil {
		logx.Errorf("events: nak error: %v", err)
	}
		return
	}

	switch action {
	case Ack:
		if err := m.Ack(); err != nil {
		logx.Errorf("events: ack error: %v", err)
	}
	case Nak:
		if err := m.Nak(); err != nil {
		logx.Errorf("events: nak error: %v", err)
	}
	case NakDelay:
		if err := m.NakWithDelay(getNakDelay(cfg)); err != nil {
		logx.Errorf("events: nak delay error: %v", err)
	}
	case Term:
		if err := m.Term(); err != nil {
		logx.Errorf("events: term error: %v", err)
	}
	default:
		if err := m.Ack(); err != nil {
		logx.Errorf("events: ack error: %v", err)
	}
	}
}
