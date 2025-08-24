package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
)

// Attachment represents the structure for message attachments
type Attachment struct {
	ContentType     string  `json:"contentType"`
	Filename        *string `json:"filename"`
	ID              string  `json:"id"`
	Size            int     `json:"size"`
	Width           int     `json:"width"`
	Height          int     `json:"height"`
	Caption         *string `json:"caption"`
	UploadTimestamp int     `json:"uploadTimestamp"`
}

// SentMessage represents the sent message structure
type SentMessage struct {
	Destination       string        `json:"destination"`
	DestinationNumber string        `json:"destinationNumber"`
	DestinationUUID   string        `json:"destinationUuid"`
	Timestamp         int64         `json:"timestamp"`
	Message           *string       `json:"message"`
	ExpiresInSeconds  int           `json:"expiresInSeconds"`
	ViewOnce          bool          `json:"viewOnce"`
	Attachments       *[]Attachment `json:"attachments"`
}

// SyncMessage represents the sync message structure
type SyncMessage struct {
	SentMessage *SentMessage `json:"sentMessage"`
}

// Envelope represents the envelope structure
type Envelope struct {
	Source                   string      `json:"source"`
	SourceNumber             string      `json:"sourceNumber"`
	SourceUUID               string      `json:"sourceUuid"`
	SourceName               string      `json:"sourceName"`
	SourceDevice             int         `json:"sourceDevice"`
	Timestamp                int64       `json:"timestamp"`
	ServerReceivedTimestamp  int64       `json:"serverReceivedTimestamp"`
	ServerDeliveredTimestamp int64       `json:"serverDeliveredTimestamp"`
	SyncMessage              SyncMessage `json:"syncMessage"`
}

// EventData represents the top-level event data structure
type EventData struct {
	Envelope Envelope `json:"envelope"`
	Account  string   `json:"account"`
}

// EventDataNats represents the NATS payload structure
type EventDataNats struct {
	ID              string     `json:"id"`
	Server          string     `json:"server"`
	TimestampServer int64      `json:"timestamp"`
	Message         *EventData `json:"eventData"`
}

// Configurable variables
var (
	sseHost     = "localhost"
	ssePort     = 8080
	natsServer  = "nats://localhost:4222"
	natsSubject = "signal.inbound"
	sseURL      = fmt.Sprintf("http://%s:%d/api/v1/events", sseHost, ssePort)
	serverName  = fmt.Sprintf("%s:%d", sseHost, ssePort)
)

// receiveSignalMessages contains the main logic for processing SSE events and publishing to NATS
func receiveSignalMessages() {
	// Get the local hostname
	hostName, err := os.Hostname()
	if err != nil {
		log.Fatalf("Error getting hostname: %v", err)
	}

	// Log configurations
	log.Printf("Configurations applied:")
	log.Printf("  SSE URL: %s", sseURL)
	log.Printf("  NATS Server: %s", natsServer)
	log.Printf("  NATS Subject: %s", natsSubject)
	log.Printf("  Server Name: %s", serverName)
	log.Printf("  Local hostname: %s", hostName)

	// Initialize NATS client
	nc, err := nats.Connect(natsServer)
	if err != nil {
		log.Fatalf("Error connecting to NATS: %v", err)
	}
	defer nc.Close()

	// Initialize HTTP client for SSE
	client := &http.Client{}
	req, err := http.NewRequest("GET", sseURL, nil)
	if err != nil {
		log.Fatalf("Error creating SSE request: %v", err)
	}

	// Set headers for SSE
	req.Header.Set("Accept", "text/event-stream")
	resp, err := client.Do(req)
	if err != nil {
		log.Fatalf("Error connecting to SSE endpoint: %v", err)
	}
	defer resp.Body.Close()

	log.Printf("Connected to SSE endpoint: %s", sseURL)

	// Read SSE stream
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || !strings.HasPrefix(line, "data:") {
			continue
		}

		// Extract JSON data from SSE event
		dataStr := strings.TrimPrefix(line, "data:")

		var eventData EventData
		if err := json.Unmarshal([]byte(dataStr), &eventData); err != nil {
			log.Printf("Error validating event data: %v", err)
			continue
		}

		// Extract fields for logging with nil checks
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

		log.Printf("Message received:")
		log.Printf("  Account: %s", account)
		log.Printf("  From: %s (%s)", sourceNumber, sourceName)
		log.Printf("  Content: %s", messageContent)
		log.Printf("  TimestampSignal: %d", timestamp)

		// Create EventDataNats payload with unique ID
		eventDataNats := EventDataNats{
			ID:              uuid.NewString(),
			Server:          hostName,
			TimestampServer: time.Now().UnixMilli(),
			Message:         &eventData,
		}
		eventDataNatsJSON, err := json.Marshal(eventDataNats)
		if err != nil {
			log.Printf("Error serializing EventDataNats to JSON: %v", err)
			continue
		}

		// Publish EventDataNats to NATS
		if err := nc.Publish(natsSubject, eventDataNatsJSON); err != nil {
			log.Printf("Error publishing message to NATS: %v", err)
		} else {
			log.Printf("Message published to NATS topic '%s': %s", natsSubject, string(eventDataNatsJSON))
		}
	}

	if err := scanner.Err(); err != nil {
		log.Fatalf("Error reading SSE stream: %v", err)
	}
}

func main() {
	// Start the event processing in a goroutine
	go receiveSignalMessages()

	// Keep the main goroutine running
	select {}
}
