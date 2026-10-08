package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	// use consumer from processor module
	"example.com/payments/processor/internal/consumer"
	// use store from API module
	"example.com/payments/internal/store"
)
func main() {
    // Load config from environment
    consumerBackend := os.Getenv("CONSUMER_BACKEND")
    if consumerBackend == "" {
        consumerBackend = "kinesis"
    }

    region := os.Getenv("AWS_REGION")
    if region == "" {
        region = "us-west-2"
    }

    tableIdempotency := os.Getenv("DDB_TABLE_IDEMPOTENCY")
    tablePayments := os.Getenv("DDB_TABLE_PAYMENTS")
    tableBalances := os.Getenv("DDB_TABLE_BALANCES")

    log.Printf("Starting payments-processor...")
    log.Printf("Consumer backend: %s", consumerBackend)
    log.Printf("DynamoDB: region=%s", region)

    // Initialize DynamoDB store
    ctx := context.Background()
    ddbStore, err := store.NewDynamo(ctx, store.DynamoConfig{
        Region:           region,
        TableIdempotency: tableIdempotency,
        TablePayments:    tablePayments,
        TableBalances:    tableBalances,
    })
    if err != nil {
        log.Fatalf("Failed to initialize DynamoDB store: %v", err)
    }

    // Setup signal handling for graceful shutdown
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

    // Initialize consumer based on backend
    var consumerShutdown func() error

    if consumerBackend == "kinesis" {
        streamName := os.Getenv("KINESIS_STREAM_NAME")
        checkpointTable := os.Getenv("KINESIS_CHECKPOINT_TABLE")
        groupID := os.Getenv("CONSUMER_GROUP_ID")
        if groupID == "" {
            groupID = "payments-processor"
        }

        log.Printf("Kinesis: stream=%s, checkpoint=%s, group=%s", streamName, checkpointTable, groupID)

        kinesisConsumer, err := consumer.NewKinesisConsumer(consumer.KinesisConsumerConfig{
            StreamName:      streamName,
            ConsumerGroup:   groupID,
            CheckpointTable: checkpointTable,
            Store:           ddbStore,
        })
        if err != nil {
            log.Fatalf("Failed to initialize Kinesis consumer: %v", err)
        }

        // Start consumer in goroutine
        go func() {
            if err := kinesisConsumer.Start(); err != nil {
                log.Printf("Consumer error: %v", err)
            }
        }()

        consumerShutdown = kinesisConsumer.Shutdown

    } else if consumerBackend == "kafka" {
        bootstrapServers := os.Getenv("KAFKA_BOOTSTRAP_SERVERS")
        topic := os.Getenv("KAFKA_TOPIC")
        if topic == "" {
            topic = "payments.events"
        }
        groupID := os.Getenv("KAFKA_GROUP_ID")
        if groupID == "" {
            groupID = "payments-processor"
        }

        log.Printf("Kafka: brokers=%s, topic=%s, group=%s", bootstrapServers, topic, groupID)

        kafkaConsumer, err := consumer.NewKafkaConsumer(consumer.ConsumerConfig{
            BootstrapServers: bootstrapServers,
            Topic:            topic,
            GroupID:          groupID,
            Store:            ddbStore,
        })
        if err != nil {
            log.Fatalf("Failed to initialize Kafka consumer: %v", err)
        }

        // Start consumer in goroutine
        go func() {
            if err := kafkaConsumer.Start(); err != nil {
                log.Printf("Consumer error: %v", err)
            }
        }()

        consumerShutdown = kafkaConsumer.Shutdown

    } else {
        log.Fatalf("Unknown consumer backend: %s", consumerBackend)
    }

    // Wait for shutdown signal
    sig := <-sigChan
    log.Printf("Received signal: %v", sig)

    // Graceful shutdown
    if err := consumerShutdown(); err != nil {
        log.Printf("Error during shutdown: %v", err)
    }

    log.Println("Payments processor stopped")
}