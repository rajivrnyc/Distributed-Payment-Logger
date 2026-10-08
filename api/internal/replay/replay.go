package replay

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/segmentio/kafka-go"

	"example.com/payments/internal/core"
	"example.com/payments/internal/store"
)

type Config struct {
	Mode             string
	Topic            string
	BootstrapServers string
	BatchSize        int
}

type ReplayTool struct {
	config   Config
	reader   *kafka.Reader
	store    *store.DynamoStore
	handlers map[core.EventType]EventHandler
	metrics  *Metrics
}

type Metrics struct {
	EventsProcessed int64
	EventsSkipped   int64
	EventsFailed    int64
	StartTime       time.Time
	EventsByType    map[core.EventType]int64
}

func New(config Config, store *store.DynamoStore) (*ReplayTool, error) {
	// Parse bootstrap servers
	brokers := strings.Split(config.BootstrapServers, ",")
	if len(brokers) == 0 || brokers[0] == "" {
		return nil, fmt.Errorf("empty bootstrap servers")
	}

	// Create Kafka reader
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:     brokers,
		Topic:       config.Topic,
		GroupID:     fmt.Sprintf("replay-tool-%d", time.Now().Unix()),
		StartOffset: kafka.FirstOffset, // Read from beginning
		MinBytes:    1,
		MaxBytes:    10e6, // 10MB
		MaxWait:     500 * time.Millisecond,
	})

	return &ReplayTool{
		config:   config,
		reader:   reader,
		store:    store,
		handlers: initHandlers(store),
		metrics: &Metrics{
			StartTime:    time.Now(),
			EventsByType: make(map[core.EventType]int64),
		},
	}, nil
}

func (r *ReplayTool) Run(ctx context.Context) error {
	defer r.reader.Close()

	log.Printf("Starting replay from topic: %s", r.config.Topic)
	log.Printf("Bootstrap servers: %s", r.config.BootstrapServers)
	log.Println("Reading from earliest offset...")

	consecutiveTimeouts := 0
	maxTimeouts := 10 // Stop after 10 consecutive timeouts

	// Main replay loop
	for {
		select {
		case <-ctx.Done():
			log.Println("Context cancelled, stopping replay")
			return r.printSummary()

		default:
			// Read message with timeout
			readCtx, cancel := context.WithTimeout(ctx, 1*time.Second)
			msg, err := r.reader.ReadMessage(readCtx)
			cancel()

			if err != nil {
				if err == context.DeadlineExceeded {
					consecutiveTimeouts++
					if consecutiveTimeouts >= maxTimeouts {
						log.Println("No more messages available (timeout limit reached)")
						return r.printSummary()
					}
					continue
				}
				return fmt.Errorf("read message failed: %w", err)
			}

			// Reset counter on successful read
			consecutiveTimeouts = 0

			// Process the event
			if err := r.processEvent(msg.Value); err != nil {
				log.Printf("Failed to process event at offset %d: %v", msg.Offset, err)
				r.metrics.EventsFailed++
				continue
			}

			// Log progress every 100 events
			if r.metrics.EventsProcessed%100 == 0 {
				elapsed := time.Since(r.metrics.StartTime)
				rate := float64(r.metrics.EventsProcessed) / elapsed.Seconds()
				log.Printf("Progress: %d events processed (%.0f events/sec)",
					r.metrics.EventsProcessed, rate)
			}
		}
	}
}

func (r *ReplayTool) processEvent(msgValue []byte) error {
	var event core.Event
	if err := json.Unmarshal(msgValue, &event); err != nil {
		return fmt.Errorf("unmarshal failed: %w", err)
	}

	// Get handler for this event type
	handler, exists := r.handlers[event.EventType]
	if !exists {
		r.metrics.EventsSkipped++
		return fmt.Errorf("no handler for event type: %s", event.EventType)
	}

	// Handle event idempotently
	if err := handler.Handle(event); err != nil {
		return fmt.Errorf("handler failed for event %s: %w", event.EventID, err)
	}

	// Update metrics
	r.metrics.EventsProcessed++
	r.metrics.EventsByType[event.EventType]++

	return nil
}

func (r *ReplayTool) printSummary() error {
	duration := time.Since(r.metrics.StartTime)

	log.Println("\n" + strings.Repeat("=", 60))
	log.Println("REPLAY SUMMARY")
	log.Println(strings.Repeat("=", 60))
	log.Printf("Duration: %s", duration)
	log.Printf("Events Processed: %d", r.metrics.EventsProcessed)
	log.Printf("Events Skipped: %d", r.metrics.EventsSkipped)
	log.Printf("Events Failed: %d", r.metrics.EventsFailed)

	if duration.Seconds() > 0 {
		log.Printf("Average Rate: %.2f events/sec",
			float64(r.metrics.EventsProcessed)/duration.Seconds())
	}

	log.Println("\nEvents by Type:")
	for eventType, count := range r.metrics.EventsByType {
		log.Printf("  %-30s: %d", eventType, count)
	}
	log.Println(strings.Repeat("=", 60))

	return nil
}

func initHandlers(store *store.DynamoStore) map[core.EventType]EventHandler {
	return map[core.EventType]EventHandler{
		core.EvtPaymentRequested:  NewPaymentRequestedHandler(store),
		core.EvtPaymentAuthorized: NewPaymentAuthorizedHandler(store),
		core.EvtPaymentCaptured:   NewPaymentCapturedHandler(store),
		core.EvtPaymentFailed:     NewPaymentFailedHandler(store),
		core.EvtAuthRequested:     NewAuthRequestedHandler(store),
		core.EvtCaptureRequested:  NewCaptureRequestedHandler(store),
		core.EvtFailedManually:    NewFailedManuallyHandler(store),
	}
}
