package main

import (
	"context"
	"encoding/json"
	"log"

	_ "github.com/go-sql-driver/mysql" // Import the MySQL driver
	"github.com/nats-io/nats.go"
)

// startHistoryNatsSubscribers configures and starts NATS subscribers dedicated to logging messages to history.
func startHistoryNatsSubInbound(ctx context.Context, nc *nats.Conn, cfg *Config) {

	historyDB, err := ConnectHistoryDB(cfg.MySQLDSN)
	if err != nil {
		log.Fatalf("Could not connect to history database: %v", err)
	}
	defer func() {
		if closeErr := historyDB.Close(); closeErr != nil {
			log.Printf("Error closing history database: %v", closeErr)
		}
	}()

	// Subscriber for inbound messages
	// Subscribes to the standard NATS topic where `receiveSignalMessageService` (in another service) publishes.
	// Using Subscribe for scalability among multiple history logger instances.
	inboundSub, err := nc.Subscribe(cfg.NatsSubjectIn, func(msg *nats.Msg) {

		log.Printf("Received INBOUND NATS message for history on topic '%s'.", msg.Subject)
		var inboundPayload InboundNatsMessagePayload
		if err := json.Unmarshal(msg.Data, &inboundPayload); err != nil {
			log.Printf("Error decoding INBOUND NATS payload for history: %v", err)
			return
		}

		if inboundPayload.Server == getHostname() {
			if err := historyDB.insertInboundMessage(&inboundPayload, getHostname()); err != nil {
				log.Printf("Error logging INBOUND message to history: %v", err)
			}
		}

	})
	if err != nil {
		log.Fatalf("Failed to subscribe to topic '%s' for inbound history: %v", cfg.NatsSubjectIn, err)
	}
	log.Printf("Successfully subscribed to topic '%s' for inbound history logging.", cfg.NatsSubjectIn)

	// Wait for the context to be cancelled
	<-ctx.Done()
	log.Println("Context cancelled, shutting down OUTBOUND history subscriber.")

	if err := inboundSub.Unsubscribe(); err != nil {
		log.Printf("Error unsubscribing from INBOUND topic: %v", err)
	}

}
