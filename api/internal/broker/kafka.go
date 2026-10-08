package broker

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"log"
	"strings"
	"time"

	"example.com/payments/internal/core"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/segmentio/kafka-go"
	"github.com/segmentio/kafka-go/sasl/aws_msk_iam_v2"
)

type KafkaBroker struct {
	writer *kafka.Writer
	topic  string
}

func NewKafka(bootstrapServers, topic string) Broker {
	brokers := strings.Split(bootstrapServers, ",")
	if len(brokers) == 0 || brokers[0] == "" {
		log.Fatal("KafkaBroker: empty bootstrap servers")
	}

	// MSK IAM authentication
	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		log.Fatalf("KafkaBroker: failed to load AWS config: %v", err)
	}

	mechanism := aws_msk_iam_v2.NewMechanism(cfg)

	transport := &kafka.Transport{
		SASL: mechanism,
		TLS:  &tls.Config{},
	}

	writer := &kafka.Writer{
		Addr:         kafka.TCP(brokers...),
		Topic:        topic,
		Balancer:     &kafka.Hash{},    // ensures same accountId → same partition
		RequiredAcks: kafka.RequireAll, // safe write
		Async:        false,
		Transport:    transport,
	}

	return &KafkaBroker{
		writer: writer,
		topic:  topic,
	}
}

func (k *KafkaBroker) Publish(e core.Event) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	bytes, err := json.Marshal(e)
	if err != nil {
		log.Printf("KafkaBroker: marshal error for event %s: %v", e.EventID, err)
		return err
	}

	msg := kafka.Message{
		Key:   []byte(e.AccountID), // partitions by account
		Value: bytes,
	}

	if err := k.writer.WriteMessages(ctx, msg); err != nil {
		log.Printf("KafkaBroker: write failed for event %s: %v", e.EventID, err)
		return err
	}
	return nil
}
