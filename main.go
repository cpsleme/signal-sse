package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

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

// Configurable variables
var (
	sseHost     = "localhost"
	ssePort     = 8080
	natsServer  = "nats://localhost:4222"
	natsSubject = "signal.inbound"
	sseURL      = fmt.Sprintf("http://%s:%d/api/v1/events", sseHost, ssePort)
)

func main() {
	// Log configurations
	log.Printf("Configurations applied:")
	log.Printf("  SSE URL: %s", sseURL)
	log.Printf("  NATS Server: %s", natsServer)
	log.Printf("  NATS Subject: %s", natsSubject)

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
		log.Printf("  Timestamp: %d", timestamp)

		// Publish the entire EventData to NATS
		if err := nc.Publish(natsSubject, []byte(dataStr)); err != nil {
			log.Printf("Error publishing message to NATS: %v", err)
		} else {
			log.Printf("Message published to NATS topic '%s': %s", natsSubject, dataStr)
		}
	}

	if err := scanner.Err(); err != nil {
		log.Fatalf("Error reading SSE stream: %v", err)
	}
}
