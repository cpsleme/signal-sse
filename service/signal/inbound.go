package signal_service

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"signal-sse/config"
	"signal-sse/domain"
	"signal-sse/util"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// receiveSignalMessageService is a long-running goroutine that connects to the signal-cli's
// SSE stream and publishes received messages to a NATS topic.
func ReceiveSignalMessageService(ctx context.Context, nc *nats.Conn, cfg *config.Config) {

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

	kv, _ := js.CreateKeyValue(ctx, jetstream.KeyValueConfig{Bucket: "signal_history_bkt"})

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}

		var eventData domain.EventDataIn
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data:")), &eventData); err != nil {
			log.Printf("Error decoding event data: %v", err)
			continue
		}

		idMessage := fmt.Sprintf("%s-%v", eventData.Envelope.SourceUUID, eventData.Envelope.Timestamp)

		inboundPayload := domain.InboundNatsMessagePayload{
			ID:              idMessage,
			Server:          util.GetHostname(),
			TimestampServer: time.Now().UnixMilli(),
			EventData:       &eventData,
		}

		payloadBytes, err := json.Marshal(inboundPayload)
		if err != nil {
			log.Printf("Error serializing NATS payload: %v", err)
			continue
		}

		// Put into Jetstream Bucket
		_, err = kv.Put(ctx, idMessage, payloadBytes)

		if err != nil {
			log.Printf("Error putting to Jetstream KV: %v", err)
		}

		// Publish to topic
		if err := nc.Publish(cfg.NatsSubjectIn, payloadBytes); err != nil {
			log.Printf("Error publishing to NATS: %v", err)
		} else {
			log.Printf("Published message '%s' from '%s' to '%s'.", idMessage, eventData.Envelope.SourceNumber, cfg.NatsSubjectIn)
		}

	}

	if err := scanner.Err(); err != nil {
		log.Printf("SSE stream error: %v", err)
	}
	log.Println("SSE receiver shut down.")
}
