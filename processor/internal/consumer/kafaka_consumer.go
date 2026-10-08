package consumer

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"example.com/payments/internal/core"
	"example.com/payments/internal/store"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/segmentio/kafka-go"
	"github.com/segmentio/kafka-go/sasl/aws_msk_iam_v2"
)

type KafkaConsumer struct {
    reader       *kafka.Reader
    store        *store.DynamoStore
    groupID      string
    topic        string
    mu            sync.Mutex
    ctx           context.Context
    cancel        context.CancelFunc
}

type ConsumerConfig struct {
    BootstrapServers string
    Topic            string
    GroupID          string
    Store            *store.DynamoStore
}

func NewKafkaConsumer(cfg ConsumerConfig) (*KafkaConsumer, error) {
    if cfg.Store == nil {
        return nil, fmt.Errorf("store cannot be nil")
    }

    // MSK IAM authentication
    awsCfg, err := config.LoadDefaultConfig(context.Background())
    if err != nil {
        return nil, fmt.Errorf("failed to load AWS config: %w", err)
    }

    mechanism := aws_msk_iam_v2.NewMechanism(awsCfg)

    dialer := &kafka.Dialer{
        Timeout:       10 * time.Second,
        DualStack:     true,
        SASLMechanism: mechanism,
        TLS:           &tls.Config{},
    }

    reader := kafka.NewReader(kafka.ReaderConfig{
        Brokers:        []string{cfg.BootstrapServers},
        Topic:          cfg.Topic,
        GroupID:        cfg.GroupID,
        StartOffset:    kafka.LastOffset,
        MaxBytes:       10e6, // 10MB
        CommitInterval: 1 * time.Second,
        Dialer:         dialer,
    })

    ctx, cancel := context.WithCancel(context.Background())

    return &KafkaConsumer{
        reader:   reader,
        store:    cfg.Store,
        groupID:  cfg.GroupID,
        topic:    cfg.Topic,
        ctx:      ctx,
        cancel:   cancel,
    }, nil
}

// Start begins consuming events from Kafka
func (kc *KafkaConsumer) Start() error {
    log.Printf("Kafka consumer started: group=%s, topic=%s", kc.groupID, kc.topic)

    for {
        select {
        case <-kc.ctx.Done():
            log.Println("Kafka consumer shutting down...")
            return kc.ctx.Err()
        default:
        }

        msg, err := kc.reader.ReadMessage(kc.ctx)
        if err != nil {
            log.Printf("Error reading message: %v", err)
            continue
        }

        if err := kc.handleMessage(msg); err != nil {
            log.Printf("Error handling message (offset=%d): %v", msg.Offset, err)
            // TODO: Send to DLQ for poison events
        }
    }
}

func (kc *KafkaConsumer) handleMessage(msg kafka.Message) error {
    var evt core.Event
    if err := json.Unmarshal(msg.Value, &evt); err != nil {
        log.Printf("Failed to unmarshal event: %v", err)
        return err
    }

    log.Printf("Processing event: id=%s, type=%s, account=%s, payment=%s", 
        evt.EventID, evt.EventType, evt.AccountID, evt.PaymentID)

    // Route event to appropriate handler
    switch evt.EventType {
    case core.EvtPaymentRequested:
        return kc.handlePaymentRequested(evt)
    case core.EvtPaymentAuthorized:
        return kc.handlePaymentAuthorized(evt)
    case core.EvtPaymentCaptured:
        return kc.handlePaymentCaptured(evt)
    case core.EvtPaymentFailed:
        return kc.handlePaymentFailed(evt)
    default:
        log.Printf("Unknown event type: %s", evt.EventType)
        return nil
    }
}

func (kc *KafkaConsumer) handlePaymentRequested(evt core.Event) error {
    payload := core.MustPayload(evt.Payload)

    // Upsert payment to REQUESTED state
    if err := kc.store.PaymentUpsertRequested(
        kc.ctx,
        evt.PaymentID,
        evt.AccountID,
        payload.Amount,
        payload.Currency,
        evt.EventID,
    ); err != nil {
        log.Printf("Failed to upsert payment: %v", err)
        return err
    }

    // Increment pending balance
    if err := kc.store.BalanceIncPending(
        kc.ctx,
        evt.AccountID,
        payload.Currency,
        evt.EventID,
        payload.Amount,
    ); err != nil {
        log.Printf("Failed to increment pending balance: %v", err)
        return err
    }

    log.Printf("Payment requested processed: paymentId=%s, amount=%d", evt.PaymentID, payload.Amount)
    return nil
}

func (kc *KafkaConsumer) handlePaymentAuthorized(evt core.Event) error {
    // Mark payment as authorized
    if err := kc.store.PaymentMarkAuthorized(kc.ctx, evt.PaymentID, evt.EventID, evt.OccurredAt); err != nil {
        log.Printf("Failed to mark payment authorized: %v", err)
        return err
    }

    log.Printf("Payment authorized processed: paymentId=%s", evt.PaymentID)
    return nil
}

func (kc *KafkaConsumer) handlePaymentCaptured(evt core.Event) error {
    payload := core.MustPayload(evt.Payload)

    // Mark payment as captured
    if err := kc.store.PaymentMarkCaptured(kc.ctx, evt.PaymentID, evt.EventID, evt.OccurredAt); err != nil {
        log.Printf("Failed to mark payment captured: %v", err)
        return err
    }

    // Move pending balance to available
    if err := kc.store.BalanceMovePendingToAvailable(
        kc.ctx,
        evt.AccountID,
        evt.EventID,
        payload.CapturedAmount,
    ); err != nil {
        log.Printf("Failed to move balance: %v", err)
        return err
    }

    log.Printf("Payment captured processed: paymentId=%s, amount=%d", evt.PaymentID, payload.CapturedAmount)
    return nil
}

func (kc *KafkaConsumer) handlePaymentFailed(evt core.Event) error {
    payload := core.MustPayload(evt.Payload)

    // Mark payment as failed
    if err := kc.store.PaymentMarkFailed(kc.ctx, evt.PaymentID, evt.EventID, payload.Reason); err != nil {
        log.Printf("Failed to mark payment failed: %v", err)
        return err
    }

    // Decrement pending balance (refund)
    if err := kc.store.BalanceDecPending(
        kc.ctx,
        evt.AccountID,
        evt.EventID,
        payload.Amount,
    ); err != nil {
        log.Printf("Failed to decrement pending balance: %v", err)
        return err
    }

    log.Printf("Payment failed processed: paymentId=%s, reason=%s", evt.PaymentID, payload.Reason)
    return nil
}

// Shutdown gracefully stops the consumer
func (kc *KafkaConsumer) Shutdown() error {
    log.Println("Shutting down Kafka consumer...")
    kc.cancel()
    return kc.reader.Close()
}