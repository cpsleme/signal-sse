package infra

import (
	"context"
	"log"
	"signal-sse/config"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// Centralized NATS connection
func ConnectToNATS(ctx context.Context, cfg *config.Config) (nc *nats.Conn, kv jetstream.KeyValue) {

	log.Printf("Attempting to connect to NATS at: %s", cfg.NatsServer)

	nc, err := nats.Connect(cfg.NatsServer)
	if err != nil {
		log.Fatalf("Could not connect to NATS. %v", err)
	}
	defer nc.Close()

	js, err := jetstream.New(nc)
	if err != nil {
		log.Fatalf("Could not create JetStream context. %v", err)
	}

	kv, _ = js.CreateKeyValue(ctx, jetstream.KeyValueConfig{Bucket: "signal_history_bkt"})

	log.Printf("Successfully connected to NATS at: %s", cfg.NatsServer)

	return nc, kv
}
