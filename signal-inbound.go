package main

import (
	"bufio"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
)

// receiveSignalMessageService is a long-running goroutine that connects to the signal-cli's
// SSE stream and publishes received messages to a NATS topic.
func receiveSignalMessageService(ctx context.Context, nc *nats.Conn, cfg *Config) {

	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)

	log.Printf("receiveSignalMessageService: Starting with configurations:")
	log.Printf("  SSE URL: %s", cfg.SSEURLReceive)
	log.Printf("  NATS Server: %s", cfg.NatsServer)
	log.Printf("  NATS Subject: %s", cfg.NatsSubjectIn)
	log.Printf("  Local hostname: %s", getHostname())

	client := &http.Client{Timeout: 30 * time.Second}                          // Add a timeout for the SSE client
	req, err := http.NewRequestWithContext(ctx, "GET", cfg.SSEURLReceive, nil) // Use context with request
	if err != nil {
		log.Fatalf("Fatal Error: receiveSignalMessageService: Error creating SSE request: %v", err)
	}
	req.Header.Set("Accept", "text/event-stream")

	resp, err := client.Do(req)
	if err != nil {
		log.Fatalf("Fatal Error: receiveSignalMessageService: Error connecting to SSE endpoint: %v", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			log.Printf("receiveSignalMessageService: Error closing SSE response body: %v", closeErr)
		}
	}()

	log.Printf("receiveSignalMessageService: Connected to SSE endpoint: %s", cfg.SSEURLReceive)

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			log.Println("receiveSignalMessageService: Context cancelled, shutting down SSE scanner.")
			return
		default:
			line := scanner.Text()
			if line == "" || !strings.HasPrefix(line, "data:") {
				continue
			}

			dataStr := strings.TrimPrefix(line, "data:")

			var eventData EventDataIn
			if err := json.Unmarshal([]byte(dataStr), &eventData); err != nil {
				log.Printf("receiveSignalMessageService: Error validating event data: %v", err)
				continue
			}

			account := eventData.Account
			sourceNumber := eventData.Envelope.SourceNumber
			sourceName := eventData.Envelope.SourceName
			timestamp := eventData.Envelope.Timestamp
			var messageContent string
			if eventData.Envelope.SyncMessage.SentMessage != nil && eventData.Envelope.SyncMessage.SentMessage.Message != nil {
				messageContent = *eventData.Envelope.SyncMessage.SentMessage.Message
			} else {
				messageContent = "<no message content>"
			}

			log.Printf("receiveSignalMessageService: Message received:")
			log.Printf("  Account: %s", account)
			log.Printf("  From: %s (%s)", sourceNumber, sourceName)
			log.Printf("  Content: %s", messageContent)
			log.Printf("  TimestampSignal: %d", timestamp)

			inboundPayload := InboundNatsMessagePayload{
				ID:              uuid.NewString(),
				Server:          getHostname(),
				TimestampServer: time.Now().UnixMilli(),
				EventData:       &eventData,
			}

			inboundPayloadJSON, err := json.Marshal(inboundPayload)
			if err != nil {
				log.Printf("receiveSignalMessageService: Error serializing InboundNatsMessagePayload to JSON: %v", err)
				continue
			}

			if err := nc.Publish(cfg.NatsSubjectIn, inboundPayloadJSON); err != nil {
				log.Printf("receiveSignalMessageService: Error publishing message to NATS: %v", err)
			} else {
				log.Printf("receiveSignalMessageService: Message published to NATS topic '%s'.", cfg.NatsSubjectIn)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		log.Printf("receiveSignalMessageService: Error reading SSE stream: %v", err)
	}
	log.Println("receiveSignalMessageService: SSE reader shutting down.")
}
