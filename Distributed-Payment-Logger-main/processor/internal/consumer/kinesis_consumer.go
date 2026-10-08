package consumer

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"example.com/payments/internal/core"
	"example.com/payments/internal/store"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	dynamoTypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go-v2/service/kinesis"
	kinesisTypes "github.com/aws/aws-sdk-go-v2/service/kinesis/types"
)

type KinesisConsumer struct {
	client           *kinesis.Client
	dynamoClient     *dynamodb.Client
	store            *store.DynamoStore
	streamName       string
	consumerGroup    string
	checkpointTable  string
	ctx              context.Context
	cancel           context.CancelFunc
}

type KinesisConsumerConfig struct {
	StreamName      string
	ConsumerGroup   string
	CheckpointTable string
	Store           *store.DynamoStore
}

func NewKinesisConsumer(cfg KinesisConsumerConfig) (*KinesisConsumer, error) {
	if cfg.Store == nil {
		return nil, fmt.Errorf("store cannot be nil")
	}
	if cfg.StreamName == "" {
		return nil, fmt.Errorf("stream name is required")
	}
	if cfg.ConsumerGroup == "" {
		cfg.ConsumerGroup = "payment-processor"
	}

	awsCfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	kinesisClient := kinesis.NewFromConfig(awsCfg)
	dynamoClient := dynamodb.NewFromConfig(awsCfg)

	ctx, cancel := context.WithCancel(context.Background())

	return &KinesisConsumer{
		client:          kinesisClient,
		dynamoClient:    dynamoClient,
		store:           cfg.Store,
		streamName:      cfg.StreamName,
		consumerGroup:   cfg.ConsumerGroup,
		checkpointTable: cfg.CheckpointTable,
		ctx:             ctx,
		cancel:          cancel,
	}, nil
}

// Start begins consuming events from Kinesis
func (kc *KinesisConsumer) Start() error {
	log.Printf("Kinesis consumer started: group=%s, stream=%s", kc.consumerGroup, kc.streamName)

	// Get list of shards
	shards, err := kc.listShards()
	if err != nil {
		return fmt.Errorf("failed to list shards: %w", err)
	}

	log.Printf("Found %d shards to process", len(shards))

	// Process each shard in parallel
	errChan := make(chan error, len(shards))
	for _, shard := range shards {
		go func(shardID string) {
			errChan <- kc.processShard(shardID)
		}(shard)
	}

	// Wait for any error or context cancellation
	select {
	case err := <-errChan:
		log.Printf("Shard processing error: %v", err)
		return err
	case <-kc.ctx.Done():
		log.Println("Kinesis consumer shutting down...")
		return kc.ctx.Err()
	}
}

func (kc *KinesisConsumer) listShards() ([]string, error) {
	input := &kinesis.ListShardsInput{
		StreamName: aws.String(kc.streamName),
	}

	output, err := kc.client.ListShards(kc.ctx, input)
	if err != nil {
		return nil, err
	}

	shardIDs := make([]string, 0, len(output.Shards))
	for _, shard := range output.Shards {
		shardIDs = append(shardIDs, *shard.ShardId)
	}

	return shardIDs, nil
}

func (kc *KinesisConsumer) processShard(shardID string) error {
	log.Printf("Processing shard: %s", shardID)

	// Get checkpoint for this shard
	sequenceNumber, err := kc.getCheckpoint(shardID)
	if err != nil {
		log.Printf("Failed to get checkpoint for shard %s: %v", shardID, err)
		sequenceNumber = "" // Start from beginning
	}

	// Get shard iterator
	var iteratorType kinesisTypes.ShardIteratorType
	var getIteratorInput *kinesis.GetShardIteratorInput

	if sequenceNumber != "" {
		iteratorType = kinesisTypes.ShardIteratorTypeAfterSequenceNumber
		getIteratorInput = &kinesis.GetShardIteratorInput{
			StreamName:             aws.String(kc.streamName),
			ShardId:                aws.String(shardID),
			ShardIteratorType:      iteratorType,
			StartingSequenceNumber: aws.String(sequenceNumber),
		}
	} else {
		iteratorType = kinesisTypes.ShardIteratorTypeTrimHorizon // Start from oldest
		getIteratorInput = &kinesis.GetShardIteratorInput{
			StreamName:        aws.String(kc.streamName),
			ShardId:           aws.String(shardID),
			ShardIteratorType: iteratorType,
		}
	}

	iteratorOutput, err := kc.client.GetShardIterator(kc.ctx, getIteratorInput)
	if err != nil {
		return fmt.Errorf("failed to get shard iterator: %w", err)
	}

	shardIterator := iteratorOutput.ShardIterator

	// Continuously read from shard
	for {
		select {
		case <-kc.ctx.Done():
			return kc.ctx.Err()
		default:
		}

		if shardIterator == nil {
			log.Printf("Shard %s closed or completed", shardID)
			return nil
		}

		getRecordsInput := &kinesis.GetRecordsInput{
			ShardIterator: shardIterator,
			Limit:         aws.Int32(100),
		}

		recordsOutput, err := kc.client.GetRecords(kc.ctx, getRecordsInput)
		if err != nil {
			log.Printf("Error getting records from shard %s: %v", shardID, err)
			time.Sleep(1 * time.Second)
			continue
		}

		// Process records
		for _, record := range recordsOutput.Records {
			if err := kc.handleRecord(record); err != nil {
				log.Printf("Error handling record (seq=%s): %v", *record.SequenceNumber, err)
				// Continue processing other records
			}

			// Update checkpoint
			if err := kc.saveCheckpoint(shardID, *record.SequenceNumber); err != nil {
				log.Printf("Failed to save checkpoint: %v", err)
			}
		}

		// Update iterator for next batch
		shardIterator = recordsOutput.NextShardIterator

		// Rate limiting to avoid exceeding Kinesis limits
		if len(recordsOutput.Records) == 0 {
			time.Sleep(1 * time.Second)
		}
	}
}

func (kc *KinesisConsumer) handleRecord(record kinesisTypes.Record) error {
	var evt core.Event
	if err := json.Unmarshal(record.Data, &evt); err != nil {
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

func (kc *KinesisConsumer) handlePaymentRequested(evt core.Event) error {
	payload := core.MustPayload(evt.Payload)

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

func (kc *KinesisConsumer) handlePaymentAuthorized(evt core.Event) error {
	if err := kc.store.PaymentMarkAuthorized(kc.ctx, evt.PaymentID, evt.EventID, evt.OccurredAt); err != nil {
		log.Printf("Failed to mark payment authorized: %v", err)
		return err
	}

	log.Printf("Payment authorized processed: paymentId=%s", evt.PaymentID)
	return nil
}

func (kc *KinesisConsumer) handlePaymentCaptured(evt core.Event) error {
	payload := core.MustPayload(evt.Payload)

	if err := kc.store.PaymentMarkCaptured(kc.ctx, evt.PaymentID, evt.EventID, evt.OccurredAt); err != nil {
		log.Printf("Failed to mark payment captured: %v", err)
		return err
	}

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

func (kc *KinesisConsumer) handlePaymentFailed(evt core.Event) error {
	payload := core.MustPayload(evt.Payload)

	if err := kc.store.PaymentMarkFailed(kc.ctx, evt.PaymentID, evt.EventID, payload.Reason); err != nil {
		log.Printf("Failed to mark payment failed: %v", err)
		return err
	}

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

// Checkpoint management using DynamoDB
func (kc *KinesisConsumer) getCheckpoint(shardID string) (string, error) {
	input := &dynamodb.GetItemInput{
		TableName: aws.String(kc.checkpointTable),
		Key: map[string]dynamoTypes.AttributeValue{
			"ConsumerGroup": &dynamoTypes.AttributeValueMemberS{Value: kc.consumerGroup},
			"ShardId":       &dynamoTypes.AttributeValueMemberS{Value: shardID},
		},
	}

	output, err := kc.dynamoClient.GetItem(kc.ctx, input)
	if err != nil {
		return "", err
	}

	if output.Item == nil {
		return "", nil // No checkpoint exists
	}

	if seqNum, ok := output.Item["SequenceNumber"].(*dynamoTypes.AttributeValueMemberS); ok {
		return seqNum.Value, nil
	}

	return "", nil
}

func (kc *KinesisConsumer) saveCheckpoint(shardID, sequenceNumber string) error {
	input := &dynamodb.PutItemInput{
		TableName: aws.String(kc.checkpointTable),
		Item: map[string]dynamoTypes.AttributeValue{
			"ConsumerGroup":  &dynamoTypes.AttributeValueMemberS{Value: kc.consumerGroup},
			"ShardId":        &dynamoTypes.AttributeValueMemberS{Value: shardID},
			"SequenceNumber": &dynamoTypes.AttributeValueMemberS{Value: sequenceNumber},
			"UpdatedAt":      &dynamoTypes.AttributeValueMemberS{Value: time.Now().Format(time.RFC3339)},
		},
	}

	_, err := kc.dynamoClient.PutItem(kc.ctx, input)
	return err
}

// Shutdown gracefully stops the consumer
func (kc *KinesisConsumer) Shutdown() error {
	log.Println("Shutting down Kinesis consumer...")
	kc.cancel()
	return nil
}
