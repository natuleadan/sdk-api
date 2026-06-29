package events

import (
	"context"
	"fmt"
	"time"

	"github.com/goccy/go-json"
	"github.com/nats-io/nats.go"
)

type Producer[T any] struct {
	nc *nats.Conn
	js nats.JetStreamContext
	sub string
}

func NewProducer[T any](nc *nats.Conn, js nats.JetStreamContext, subject string) *Producer[T] {
	return &Producer[T]{nc: nc, js: js, sub: subject}
}

func (p *Producer[T]) Publish(msg T, opts ...nats.PubOpt) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("events: marshal: %w", err)
	}
	_, err = p.js.Publish(p.sub, data, opts...)
	if err != nil {
		return fmt.Errorf("events: publish %s: %w", p.sub, err)
	}
	return nil
}

func (p *Producer[T]) PublishWithID(msg T, id string, opts ...nats.PubOpt) error {
	opts = append(opts, nats.MsgId(id))
	return p.Publish(msg, opts...)
}

// PublishAndWait publishes a message and waits for a reply (NATS request-reply).
// Returns the typed response or an error. Uses core NATS Request, not JetStream.
func (p *Producer[T]) PublishAndWait(ctx context.Context, msg T, timeout time.Duration) (*Msg[T], error) {
	data, err := json.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("events: marshal: %w", err)
	}

	natsMsg := nats.NewMsg(p.sub)
	natsMsg.Data = data

	resp, err := p.nc.RequestMsgWithContext(ctx, natsMsg)
	if err != nil {
		return nil, fmt.Errorf("events: request %s: %w", p.sub, err)
	}

	var result T
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, fmt.Errorf("events: unmarshal reply: %w", err)
	}

	return &Msg[T]{
		Data: result,
		Raw:  resp,
		Ack:  resp.Ack,
		Nak:  resp.Nak,
		NakD: resp.NakWithDelay,
		Term: resp.Term,
	}, nil
}

// PublishAndWaitRaw publishes and waits for a raw byte reply.
func (p *Producer[T]) PublishAndWaitRaw(ctx context.Context, data []byte, timeout time.Duration) (*nats.Msg, error) {
	natsMsg := nats.NewMsg(p.sub)
	natsMsg.Data = data

	resp, err := p.nc.RequestMsgWithContext(ctx, natsMsg)
	if err != nil {
		return nil, fmt.Errorf("events: request %s: %w", p.sub, err)
	}
	return resp, nil
}
