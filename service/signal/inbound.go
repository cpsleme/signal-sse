package signal_service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"signal-sse/config"
	"signal-sse/domain"
	"signal-sse/infra"
	"signal-sse/util"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// ReceiveSignalMessageService is a long-running goroutine that connects to the signal-cli's
// SSE stream and publishes received messages to a NATS topic.
func ReceiveSignalMessageService(ctx context.Context, nc *nats.Conn, kv jetstream.KeyValue, cfg *config.Config) {

	scanner := infra.ConnectToSSE(ctx, cfg)
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
