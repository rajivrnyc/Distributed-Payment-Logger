package app

import (
	"context"
	"log"
	"os"

	"example.com/payments/internal/broker"
	apihttp "example.com/payments/internal/http"
	"example.com/payments/internal/store"
)

type Server struct {
	mux    apihttp.Router
	Broker broker.Broker
	Store  store.Store
}

func NewServer() *Server {
	// choose store via env, default to memory
	st, err := selectStore()
	if err != nil {
		log.Printf("dynamo init failed, falling back to memory: %v", err)
		st = store.NewMemory()
	}

	// choose broker via env, default to in-memory
	// BROKER_BACKEND=memory|kafka
	b := selectBroker()

	// NOTE: we no longer run an in-process processor here.
	// Processor and replay will be separate services reading from the broker.

	// wire handlers and router exactly like before
	h := apihttp.NewHandlers(b, st)
	mux := apihttp.NewRouter(h)

	return &Server{mux: mux, Broker: b, Store: st}
}

func (s *Server) Router() apihttp.Router { return s.mux }

// --- helpers: store selection ---

func selectStore() (store.Store, error) {
	if getEnv("STORE_BACKEND", "memory") != "dynamo" {
		return store.NewMemory(), nil
	}

	region := os.Getenv("AWS_REGION")
	idem := os.Getenv("DDB_TABLE_IDEMPOTENCY")
	pays := os.Getenv("DDB_TABLE_PAYMENTS")
	bals := os.Getenv("DDB_TABLE_BALANCES")

	ds, err := store.NewDynamo(context.TODO(), store.DynamoConfig{
		Region:           region,
		TableIdempotency: idem,
		TablePayments:    pays,
		TableBalances:    bals,
	})
	if err != nil {
		return nil, err
	}
	log.Printf("STORE_BACKEND=dynamo → using DynamoDB tables: %s, %s, %s", idem, pays, bals)
	return ds, nil
}

// --- helpers: broker selection ---

func selectBroker() broker.Broker {
	backend := getEnv("BROKER_BACKEND", "memory")

	switch backend {
	case "kinesis":
		streamName := os.Getenv("KINESIS_STREAM_NAME")

		if streamName == "" {
			log.Fatal("BROKER_BACKEND=kinesis but KINESIS_STREAM_NAME is empty")
		}

		log.Printf("BROKER_BACKEND=kinesis → stream=%s", streamName)
		b, err := broker.NewKinesisBroker(context.TODO(), streamName)
		if err != nil {
			log.Fatalf("Failed to create Kinesis broker: %v", err)
		}
		return b

	case "kafka":
		bootstrap := os.Getenv("KAFKA_BOOTSTRAP_SERVERS")
		topic := getEnv("KAFKA_PAYMENTS_TOPIC", "payments.events")

		if bootstrap == "" {
			log.Fatal("BROKER_BACKEND=kafka but KAFKA_BOOTSTRAP_SERVERS is empty")
		}

		log.Printf("BROKER_BACKEND=kafka → bootstrap=%s topic=%s", bootstrap, topic)
		return broker.NewKafka(bootstrap, topic)

	default:
		buf := 1024
		log.Printf("BROKER_BACKEND=%s → using in-memory broker (buf=%d)", backend, buf)
		return broker.NewInMemory(buf)
	}
}

func getEnv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
