package events

import (
	"context"
	"fmt"
	"time"

	"github.com/segmentio/kafka-go"
)

type kafkaMessage struct {
	msg    *kafka.Message
	reader *kafka.Reader
}

func (m *kafkaMessage) Data() []byte    { return m.msg.Value }
func (m *kafkaMessage) Subject() string { return m.msg.Topic }

func (m *kafkaMessage) Ack() error {
	if m.reader != nil {
		return m.reader.CommitMessages(context.Background(), *m.msg)
	}
	return nil
}

func (m *kafkaMessage) Nak(_ ...time.Duration) error {
	return nil
}

func (m *kafkaMessage) Term() error {
	if m.reader != nil {
		return m.reader.CommitMessages(context.Background(), *m.msg)
	}
	return nil
}

func (m *kafkaMessage) Respond(_ []byte) error {
	return fmt.Errorf("kafka: replies not supported")
}
