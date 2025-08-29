package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// receiveSignalMessageService is a long-running goroutine that connects to the signal-cli's
// SSE stream and publishes received messages to a NATS topic.
func receiveSignalMessageService(ctx context.Context, nc *nats.Conn, cfg *Config) {

	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)

	log.Printf("Starting Signal SSE receiver on %s...", cfg.NatsSubjectIn)

	client := &http.Client{}
	req, err := http.NewRequestWithContext(ctx, "GET", cfg.SSEURLReceive, nil)
	if err != nil {
		log.Fatalf("Failed to create SSE request: %v", err)
	}
	req.Header.Set("Accept", "text/event-stream")

	resp, err := client.Do(req)
	if err != nil {
		log.Fatalf("Failed to connect to SSE endpoint: %v", err)
	}
	defer resp.Body.Close()

	log.Println("Connected to SSE stream.")

	js, err := jetstream.New(nc)
	if err != nil {
		log.Fatalf("Could not create JetStream context. %v", err)
	}

	kv, err := js.CreateKeyValue(ctx, jetstream.KeyValueConfig{Bucket: "signal.bucket.inbound"})

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}

		var eventData EventDataIn
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data:")), &eventData); err != nil {
			log.Printf("Error decoding event data: %v", err)
			continue
		}

		idMessage := fmt.Sprintf("%s-%v", eventData.Envelope.SourceUUID, eventData.Envelope.Timestamp)

		_, err = kv.PutString(ctx, idMessage, eventData.Envelope.SourceNumber)

		if err == nil {

			inboundPayload := InboundNatsMessagePayload{
				ID:              idMessage,
				Server:          getHostname(),
				TimestampServer: time.Now().UnixMilli(),
				EventData:       &eventData,
			}

			payloadBytes, err := json.Marshal(inboundPayload)
			if err != nil {
				log.Printf("Error serializing NATS payload: %v", err)
				continue
			}

			if err := nc.Publish(cfg.NatsSubjectIn, payloadBytes); err != nil {
				log.Printf("Error publishing to NATS: %v", err)
			} else {
				log.Printf("Published message from '%s' to '%s'.", eventData.Envelope.SourceNumber, cfg.NatsSubjectIn)
			}
		}

	}

	if err := scanner.Err(); err != nil {
		log.Printf("SSE stream error: %v", err)
	}
	log.Println("SSE receiver shut down.")
}
