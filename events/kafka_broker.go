package events

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/goccy/go-json"
	"github.com/natuleadan/sdk-api/infra/logx"
	"github.com/segmentio/kafka-go"
)

type KafkaBroker struct {
	name          string
	brokers       []string
	consumerGroup string
	mu            sync.Mutex
	ensured       map[string]bool
}

func NewKafkaBroker(name string, brokers []string, consumerGroup string) *KafkaBroker {
	return &KafkaBroker{
		name:          name,
		brokers:       brokers,
		consumerGroup: consumerGroup,
		ensured:       make(map[string]bool),
	}
}

func (b *KafkaBroker) Name() string { return b.name }

func (b *KafkaBroker) Publish(ctx context.Context, subject string, data []byte) error {
	b.mu.Lock()
	if !b.ensured[subject] {
		if err := b.ensureTopic(subject); err != nil {
			b.mu.Unlock()
			return err
		}
		b.ensured[subject] = true
	}
	b.mu.Unlock()

	conn, err := kafka.DialLeader(ctx, "tcp", b.brokers[0], subject, 0)
	if err != nil {
		return fmt.Errorf("kafka: dial leader: %w", err)
	}
	defer func() {
		if err := conn.Close(); err != nil {
			fmt.Printf("close error: %v\n", err)
		}
	}()

	_, err = conn.WriteMessages(kafka.Message{Value: data})
	return err
}

func (b *KafkaBroker) PublishJSON(ctx context.Context, subject string, msg any) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("events: marshal: %w", err)
	}
	return b.Publish(ctx, subject, data)
}

func (b *KafkaBroker) ensureTopic(topic string) error {
	conn, err := kafka.Dial("tcp", b.brokers[0])
	if err != nil {
		return fmt.Errorf("kafka: dial: %w", err)
	}
	defer func() {
		if err := conn.Close(); err != nil {
			fmt.Printf("close error: %v\n", err)
		}
	}()

	if err := conn.CreateTopics(kafka.TopicConfig{
		Topic:             topic,
		NumPartitions:     1,
		ReplicationFactor: 1,
	}); err != nil {
		return fmt.Errorf("kafka: create topic %s: %w", topic, err)
	}
	return nil
}

func (b *KafkaBroker) Subscribe(ctx context.Context, subject string, durable string, handler MessageHandler) (Subscription, error) {
	b.mu.Lock()
	if !b.ensured[subject] {
		if err := b.ensureTopic(subject); err != nil {
			b.mu.Unlock()
			return nil, err
		}
		b.ensured[subject] = true
	}
	b.mu.Unlock()

	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:     b.brokers,
		Topic:       subject,
		GroupID:     durable,
		MinBytes:    1,
		MaxBytes:    10e6,
		MaxWait:     1 * time.Second,
		StartOffset: kafka.FirstOffset,
	})
	go func() {
		defer func() {
			if err := reader.Close(); err != nil {
				fmt.Printf("close error: %v\n", err)
			}
		}()
		for {
			msg, err := reader.FetchMessage(ctx)
			if err != nil {
				return
			}
			km := &kafkaMessage{msg: &msg, reader: reader}
			if err := handler(ctx, km); err == nil {
				if err := km.Ack(); err != nil {
					logx.Errorf("kafka: ack error: %v", err)
				}
			}
		}
	}()
	return &kafkaSubscription{reader: reader}, nil
}

func (b *KafkaBroker) PullSubscribe(_ context.Context, _ string, _ string) (PullConsumer, error) {
	return nil, fmt.Errorf("kafka: pull subscribe not supported")
}

func (b *KafkaBroker) Request(_ context.Context, _ string, _ []byte, _ time.Duration) ([]byte, error) {
	return nil, fmt.Errorf("kafka: request-reply not supported")
}

func (b *KafkaBroker) EnsureStream(cfg StreamConfig) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	name := cfg.Name
	if b.ensured[name] {
		return nil
	}

	return b.ensureTopic(name)
}

func (b *KafkaBroker) EnsureStreams(configs ...StreamConfig) error {
	for _, cfg := range configs {
		if err := b.EnsureStream(cfg); err != nil {
			return err
		}
	}
	return nil
}

func (b *KafkaBroker) Close() error {
	return nil
}

type kafkaSubscription struct {
	reader *kafka.Reader
}

func (s *kafkaSubscription) Unsubscribe() error {
	return s.reader.Close()
}
