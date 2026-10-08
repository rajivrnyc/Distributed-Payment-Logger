package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"example.com/payments/internal/replay"
	"example.com/payments/internal/store"
)

func main() {
	// Parse command-line flags
	mode := flag.String("mode", "full", "Replay mode: full, time-bounded, incremental")
	topic := flag.String("topic", "payments.events", "Kafka topic to replay")
	brokers := flag.String("brokers", "localhost:9092", "Kafka bootstrap servers (comma-separated)")
	region := flag.String("region", "us-east-1", "AWS region")
	tablePayments := flag.String("table-payments", "", "DynamoDB Payments table name")
	tableBalances := flag.String("table-balances", "", "DynamoDB Balances table name")
	tableIdem := flag.String("table-idem", "", "DynamoDB Idempotency table name")

	flag.Parse()

	log.Println("Payment System - Replay Tool")
	log.Printf("Mode:    %s", *mode)
	log.Printf("Topic:   %s", *topic)
	log.Printf("Brokers: %s", *brokers)

	// Setup context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		log.Println("\nShutdown signal received, stopping replay...")
		cancel()
	}()

	// Initialize DynamoDB store
	dynamoConfig := store.DynamoConfig{
		Region:           *region,
		TableIdempotency: *tableIdem,
		TablePayments:    *tablePayments,
		TableBalances:    *tableBalances,
	}

	// Allow reading from environment variables
	if dynamoConfig.TablePayments == "" {
		dynamoConfig.TablePayments = os.Getenv("DDB_TABLE_PAYMENTS")
	}
	if dynamoConfig.TableBalances == "" {
		dynamoConfig.TableBalances = os.Getenv("DDB_TABLE_BALANCES")
	}
	if dynamoConfig.TableIdempotency == "" {
		dynamoConfig.TableIdempotency = os.Getenv("DDB_TABLE_IDEMPOTENCY")
	}

	dynamoStore, err := store.NewDynamo(ctx, dynamoConfig)
	if err != nil {
		log.Fatalf("Failed to create DynamoDB store: %v", err)
	}

	// Create replay tool
	config := replay.Config{
		Mode:             *mode,
		Topic:            *topic,
		BootstrapServers: *brokers,
		BatchSize:        100,
	}

	replayTool, err := replay.New(config, dynamoStore)
	if err != nil {
		log.Fatalf("Failed to create replay tool: %v", err)
	}

	// Run replay
	if err := replayTool.Run(ctx); err != nil {
		log.Fatalf("Replay failed: %v", err)
	}

	log.Println("\n✓ Replay completed successfully!")
}
