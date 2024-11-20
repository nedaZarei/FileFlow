package kafka

import (
	"github.com/segmentio/kafka-go"
)

type KafkaConfig struct {
	Brokers []string
	Topic   string
}

func NewKafkaWriter(config KafkaConfig) *kafka.Writer {
	return &kafka.Writer{
		Addr:     kafka.TCP(config.Brokers...),
		Topic:    config.Topic,
		Balancer: &kafka.LeastBytes{},
	}
}
