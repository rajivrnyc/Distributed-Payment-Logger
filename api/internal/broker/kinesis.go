package broker

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"example.com/payments/internal/core"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/kinesis"
)

type KinesisBroker struct {
	client     *kinesis.Client
	streamName string
}

func NewKinesisBroker(ctx context.Context, streamName string) (*KinesisBroker, error) {
	if streamName == "" {
		return nil, fmt.Errorf("KINESIS_STREAM_NAME is required")
	}

	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	client := kinesis.NewFromConfig(cfg)

	log.Printf("Kinesis broker initialized with stream: %s", streamName)
	return &KinesisBroker{
		client:     client,
		streamName: streamName,
	}, nil
}

func (b *KinesisBroker) Publish(event core.Event) error {
	ctx := context.Background()

	// Serialize event to JSON
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	// Use accountId as partition key for ordering within account
	partitionKey := event.AccountID
	if partitionKey == "" {
		partitionKey = event.TenantID
	}

	// Put record to Kinesis
	input := &kinesis.PutRecordInput{
		StreamName:   aws.String(b.streamName),
		Data:         data,
		PartitionKey: aws.String(partitionKey),
	}

	output, err := b.client.PutRecord(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to put record to Kinesis: %w", err)
	}

	log.Printf("Published event %s to Kinesis shard %s, sequence %s", 
		event.EventID, *output.ShardId, *output.SequenceNumber)
	return nil
}

func (b *KinesisBroker) Close() error {
	log.Println("Kinesis broker closed")
	return nil
}
