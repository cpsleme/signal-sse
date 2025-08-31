package main

import (
	"context"
	"encoding/json"
	"log"
	"time"

	_ "github.com/go-sql-driver/mysql" // Import the MySQL driver
	"github.com/nats-io/nats.go"
)

// startHistoryNatsSubscribers configures and starts NATS subscribers dedicated to logging messages to history.
func startHistoryNatsSubOutbound(ctx context.Context, nc *nats.Conn, cfg *Config) {

	historyDB, err := ConnectHistoryDB(cfg.MySQLDSN)
	if err != nil {
		log.Fatalf("Could not connect to history database: %v", err)
	}
	defer func() {
		if closeErr := historyDB.Close(); closeErr != nil {
			log.Printf("Error closing history database: %v", closeErr)
		}
	}()

	// Subscriber for outbound messages
	// Subscribes to the standard NATS topic where outbound messages are published (e.g., before going to Signal CLI API).
	// Using Subscribe for scalability among multiple history logger instances.
	outboundSub, err := nc.Subscribe(cfg.NatsSubjectOut, func(msg *nats.Msg) {
		log.Printf("Received OUTBOUND NATS message for history on topic '%s'.", msg.Subject)
		var outboundMessage SignalOutboundMessage
		if err := json.Unmarshal(msg.Data, &outboundMessage); err != nil {
			log.Printf("Error decoding OUTBOUND NATS payload for history: %v", err)
			return
		}

		// The service timestamp for the outbound message is generated here, at the time of logging.
		serviceTimestamp := time.Now().UnixMilli()
		if err := historyDB.insertOutboundMessage(&outboundMessage, serviceTimestamp, getHostname()); err != nil {
			log.Printf("Error logging OUTBOUND message to history: %v", err)
		}
	})
	if err != nil {
		log.Fatalf("Failed to subscribe to topic '%s' for outbound history: %v", cfg.NatsSubjectOut, err)
	}
	log.Printf("Successfully subscribed to topic '%s' for outbound history logging.", cfg.NatsSubjectOut)

	// Wait for the context to be cancelled
	<-ctx.Done()
	log.Println("Context cancelled, shutting down OUTBOUND history subscriber.")

	if err := outboundSub.Unsubscribe(); err != nil {
		log.Printf("Error unsubscribing from OUTBOUND topic: %v", err)
	}
}
