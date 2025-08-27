package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/nats-io/nats.go"
)

// sendSignalMessage is responsible for sending a message to signal-cli's HTTP API.
func sendSignalMessage(cfg *Config, data []byte) error {
	var natsMessage SignalOutboundMessage
	if err := json.Unmarshal(data, &natsMessage); err != nil {
		log.Printf("Error decoding JSON message from NATS: %v", err)
		return fmt.Errorf("decoding NATS message: %w", err)
	}

	signalRequest := SignalRPCRequest{
		JSONRPC: "2.0",
		Method:  "send",
		Params:  natsMessage,
		ID:      1,
	}

	payloadBytes, err := json.Marshal(signalRequest)
	if err != nil {
		log.Printf("Error serializing payload for signal-cli: %v", err)
		return fmt.Errorf("serializing Signal RPC request: %w", err)
	}

	req, err := http.NewRequest("POST", cfg.SSEURLSend, bytes.NewBuffer(payloadBytes))
	if err != nil {
		log.Printf("Error creating HTTP request: %v", err)
		return fmt.Errorf("creating HTTP request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Error sending message to signal-cli: %v", err)
		return fmt.Errorf("sending message to signal-cli: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			log.Printf("Error closing HTTP response body: %v", closeErr)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		log.Printf("Failed to send message from account '%s'. Status code: %d", natsMessage.Account, resp.StatusCode)
		return fmt.Errorf("signal-cli returned non-OK status: %d", resp.StatusCode)
	}

	log.Printf("Message sent successfully from account '%s' to recipient %s.", natsMessage.Account, natsMessage.Recipient)
	return nil
}

// sendSignalMessageService subscribes to a NATS topic and processes messages using a queue group.
func sendSignalMessageService(ctx context.Context, nc *nats.Conn, cfg *Config) {

	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)

	// Subscribe to the NATS topic with a queue group.
	// This is the core change. The NATS server ensures only one subscriber
	// in this queue group receives each message, balancing the load automatically.
	_, err := nc.QueueSubscribe(
		cfg.NatsSubjectOut,
		cfg.NatsQueueGroup,
		func(msg *nats.Msg) {
			log.Printf("sendSignalMessageService: Processing message from topic '%s'...", msg.Subject)
			// Process the message in a separate goroutine to avoid blocking the subscription loop.
			go func() {
				if err := sendSignalMessage(cfg, msg.Data); err != nil {
					log.Printf("sendSignalMessageService: Failed to send message: %v.", err)
				}
			}()
		},
	)

	if err != nil {
		log.Fatalf("Fatal Error: sendSignalMessageService: Error subscribing to topic '%s': %v", cfg.NatsSubjectOut, err)
	}

	log.Printf("sendSignalMessageService: Successfully subscribed to NATS topic '%s' in queue group '%s'. Waiting for messages...", cfg.NatsSubjectOut, cfg.NatsQueueGroup)

	<-ctx.Done() // Wait for context cancellation to signal shutdown
	log.Println("sendSignalMessageService: Shutting down NATS Core subscription.")
}
