package events

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/segmentio/kafka-go"
)

type KafkaBroker struct {
	name          string
	brokers       []string
	consumerGroup string
	writers       map[string]*kafka.Writer
	mu            sync.Mutex
	ensured       map[string]bool
}

func NewKafkaBroker(name string, brokers []string, consumerGroup string) *KafkaBroker {
	return &KafkaBroker{
		name:          name,
		brokers:       brokers,
		consumerGroup: consumerGroup,
		writers:       make(map[string]*kafka.Writer),
		ensured:       make(map[string]bool),
	}
}

func (b *KafkaBroker) Name() string { return b.name }

func (b *KafkaBroker) Publish(ctx context.Context, subject string, data []byte) error {
	writer, err := b.getWriter(subject)
	if err != nil {
		return err
	}
	return writer.WriteMessages(ctx, kafka.Message{
		Value: data,
	})
}

func (b *KafkaBroker) getWriter(topic string) (*kafka.Writer, error) {
	b.mu.Lock()
	if w, ok := b.writers[topic]; ok {
		b.mu.Unlock()
		return w, nil
	}

	if !b.ensured[topic] {
		if err := b.ensureTopic(topic); err != nil {
			b.mu.Unlock()
			return nil, err
		}
		b.ensured[topic] = true
	}

	w := &kafka.Writer{
		Addr:     kafka.TCP(b.brokers...),
		Topic:    topic,
		Balancer: &kafka.LeastBytes{},
		Async:    false,
	}
	b.writers[topic] = w
	b.mu.Unlock()
	return w, nil
}

func (b *KafkaBroker) ensureTopic(topic string) error {
	conn, err := kafka.Dial("tcp", b.brokers[0])
	if err != nil {
		return fmt.Errorf("kafka: dial: %w", err)
	}
	defer conn.Close()

	if err := conn.CreateTopics(kafka.TopicConfig{
		Topic:             topic,
		NumPartitions:     1,
		ReplicationFactor: 1,
	}); err != nil {
		return fmt.Errorf("kafka: create topic %s: %w", topic, err)
	}

	// brief wait for metadata propagation
	time.Sleep(100 * time.Millisecond)
	return nil
}

func (b *KafkaBroker) Subscribe(ctx context.Context, subject string, durable string, handler MessageHandler) (Subscription, error) {
	if !b.ensured[subject] {
		if err := b.ensureTopic(subject); err != nil {
			return nil, err
		}
		b.ensured[subject] = true
	}

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
		defer reader.Close()
		for {
			msg, err := reader.FetchMessage(ctx)
			if err != nil {
				return
			}
			km := &kafkaMessage{msg: &msg, reader: reader}
			if err := handler(ctx, km); err == nil {
				km.Ack()
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
	var lastErr error
	for topic, w := range b.writers {
		if err := w.Close(); err != nil {
			lastErr = fmt.Errorf("kafka: close writer %s: %w", topic, err)
		}
	}
	return lastErr
}

type kafkaSubscription struct {
	reader *kafka.Reader
}

func (s *kafkaSubscription) Unsubscribe() error {
	return s.reader.Close()
}

type kafkaPullConsumer struct{}

func (c *kafkaPullConsumer) Fetch(_ int, _ time.Duration) ([]Message, error) {
	return nil, fmt.Errorf("kafka: pull not supported")
}

func (c *kafkaPullConsumer) Unsubscribe() error {
	return nil
}
