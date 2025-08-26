package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
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
		ID:      1, // Consider generating a unique ID if needed
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

// sendSignalMessageService is a long-running goroutine that subscribes to a NATS topic
// and uses a worker pool to send messages to the Signal CLI API.
func sendSignalMessageService(ctx context.Context, nc *nats.Conn, cfg *Config) {

	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)

	log.Printf("sendSignalMessageService: Starting with configurations:")
	log.Printf("  SSE URL: %s", cfg.SSEURLSend)
	log.Printf("  NATS Server: %s", cfg.NatsServer)
	log.Printf("  NATS Subject: %s", cfg.NatsSubjectOut)
	log.Printf("  Local hostname: %s", getHostname())

	js, err := nc.JetStream()
	if err != nil {
		log.Fatalf("Fatal Error: sendSignalMessageService: Failed to get JetStream context: %v", err)
	}

	// Ensure the stream exists for outbound messages
	_, err = js.AddStream(&nats.StreamConfig{
		Name:     cfg.StreamOut,
		Subjects: []string{cfg.NatsSubjectOut},
		// You might want to configure other stream properties like MaxBytes, MaxMsgs, etc.
	})
	if err != nil && err != nats.ErrStreamNameAlreadyInUse {
		log.Fatalf("Fatal Error: sendSignalMessageService: Failed to add JetStream for subjects %v: %v", cfg.NatsSubjectOut, err)
	}
	log.Printf("sendSignalMessageService: JetStream '%s' ensured for subjects %v.", cfg.StreamOut, cfg.NatsSubjectOut)

	// Worker pool setup
	const numWorkers = 5                        // Number of concurrent workers
	jobs := make(chan *nats.Msg, 100)           // Buffered channel for incoming NATS JetStream messages (jobs)
	workerWg := make(chan struct{}, numWorkers) // Channel to limit concurrent workers

	var wg sync.WaitGroup // Initialize a WaitGroup

	// Start workers
	for i := range numWorkers {
		wg.Add(1)
		go func(workerID int) {
			for {
				select {
				case <-ctx.Done():
					log.Printf("sendSignalMessageService: Worker %d shutting down.", workerID)
					return
				case msg := <-jobs: // Receive *nats.Msg from the channel
					log.Printf("sendSignalMessageService: Worker %d processing message from topic '%s'...", workerID, msg.Subject)

					// Process the message and check for errors
					if err := sendSignalMessage(cfg, msg.Data); err != nil {
						log.Printf("sendSignalMessageService: Worker %d failed to send message: %v. NAKing message.", workerID, err)
						msg.NakWithDelay(time.Second * 5) // NAK with a delay for retry
					} else {
						msg.Ack() // Acknowledge the message upon successful processing
					}
					<-workerWg // Release a slot in the worker pool
				}
			}
		}(i)
	}

	// NATS JetStream subscription handler with manual acknowledgments
	// We use QueueSubscribe for a queue group, and Durable to ensure consumer state persists.
	sub, err := js.QueueSubscribe(
		cfg.NatsSubjectOut,
		cfg.QueueGroup,
		func(msg *nats.Msg) {
			select {
			case <-ctx.Done():
				log.Println("sendSignalMessageService: Context cancelled, NAKing new incoming NATS JetStream message.")
				msg.Nak() // NAK if shutting down and can't process
				return
			case workerWg <- struct{}{}: // Acquire a slot in the worker pool
				jobs <- msg // Send the nats.Msg to the job channel
				log.Printf("sendSignalMessageService: Message queued for processing from topic '%s'.", msg.Subject)
			default:
				// Worker pool full, NAK the message so JetStream can re-deliver it later.
				log.Printf("sendSignalMessageService: Worker pool is full. NAKing message from topic '%s'.", msg.Subject)
				msg.NakWithDelay(time.Second * 1) // NAK with a small delay
			}
		},
		nats.Durable(cfg.QueueGroup), // Ensure durability for the consumer group
		nats.ManualAck(),             // Crucial for explicit acknowledgments
		nats.DeliverNew(),            // Start delivering new messages (or DeliverAll, DeliverLast, etc.)
		nats.AckWait(30*time.Second), // How long to wait for an Ack before redelivery
		nats.MaxDeliver(3),           // Max delivery attempts for a message
	)

	if err != nil {
		log.Fatalf("Fatal Error: sendSignalMessageService: Error subscribing to topic '%s': %v", cfg.NatsSubjectOut, err)
	}

	log.Printf("sendSignalMessageService: Successfully subscribed to JetStream topic '%s' in queue group '%s'. Waiting for messages...", cfg.NatsSubjectOut, cfg.QueueGroup)

	<-ctx.Done() // Wait for context cancellation to signal shutdown
	log.Println("sendSignalMessageService: Shutting down NATS JetStream subscription.")

	// Unsubscribe from JetStream
	if err := sub.Unsubscribe(); err != nil {
		log.Printf("sendSignalMessageService: Error unsubscribing from JetStream: %v", err)
	}

	close(jobs) // Close the job channel to signal workers to finish
	wg.Wait()   // Wait for all workers to finish their current jobs and exit
	log.Println("sendSignalMessageService: All workers finished processing remaining jobs.")
}
