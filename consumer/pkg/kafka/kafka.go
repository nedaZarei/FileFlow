package kafka

import (
	"github.com/segmentio/kafka-go"
)

type KafkaConfig struct {
	Brokers []string
	Topic   string
}

func NewKafkaReader(config KafkaConfig, groupID string) *kafka.Reader {
	return kafka.NewReader(kafka.ReaderConfig{
		Brokers:  config.Brokers,
		Topic:    config.Topic,
		GroupID:  groupID,
		MinBytes: 10e3,
		MaxBytes: 10e6,
	})
}
