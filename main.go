package main

import (
	"bufio"
	"bytes"
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

//----------------------------------------------------------------------------------------------------
// Structs for Incoming Payloads (Signal to NATS)
//----------------------------------------------------------------------------------------------------

// Attachment represents the structure for message attachments from Signal.
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

// SentMessage represents the sent message structure within a SyncMessage.
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

// SyncMessage represents the sync message structure received from Signal.
type SyncMessage struct {
	SentMessage *SentMessage `json:"sentMessage"`
}

// Envelope represents the core message envelope containing sender and message details.
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

// EventData represents the top-level event data received from the Signal SSE stream.
type EventData struct {
	Envelope Envelope `json:"envelope"`
	Account  string   `json:"account"`
}

// EventDataNats represents the payload structure for messages published to the NATS 'inbound' topic.
// It wraps the raw Signal event data with a unique ID and server-side timestamp.
type EventDataNats struct {
	ID              string     `json:"id"`
	Server          string     `json:"server"`
	TimestampServer int64      `json:"timestamp"`
	Message         *EventData `json:"eventData"`
}

//----------------------------------------------------------------------------------------------------
// Structs for Outgoing Payloads (NATS to Signal)
//----------------------------------------------------------------------------------------------------

// ResponseNatsMessage represents the JSON payload received from the NATS 'outbound' topic.
type ResponseNatsMessage struct {
	Recipient  string `json:"recipient"`
	Message    string `json:"message"`
	Attachment string `json:"attachment,omitempty"`
	Account    string `json:"account"`
}

// SignalResponseParams represents the nested 'params' structure in the JSON-RPC payload
// that is sent to the signal-cli 'send' method.
type SignalResponseParams struct {
	Recipient  string `json:"recipient"`
	Message    string `json:"message"`
	Attachment string `json:"attachment,omitempty"`
	Account    string `json:"account"`
}

// ResponseSignalMessage represents the full JSON-RPC payload to be sent to signal-cli's API.
type ResponseSignalMessage struct {
	JSONRPC string               `json:"jsonrpc"`
	Method  string               `json:"method"`
	Params  SignalResponseParams `json:"params"`
	ID      int                  `json:"id"`
}

//----------------------------------------------------------------------------------------------------
// Service Configuration
//----------------------------------------------------------------------------------------------------

var (
	sseHost        = "localhost"
	ssePort        = 8080
	natsServer     = "nats://localhost:4222"
	natsSubjectIn  = "signal.inbound"  // NATS topic for incoming Signal messages
	natsSubjectOut = "signal.outbound" // NATS topic for outgoing Signal messages
	sseURLReceive  = fmt.Sprintf("http://%s:%d/api/v1/events", sseHost, ssePort)
	sseURLSend     = fmt.Sprintf("http://%s:%d/api/v1/rpc", sseHost, ssePort)
	serverName     = fmt.Sprintf("%s:%d", sseHost, ssePort)
)

//----------------------------------------------------------------------------------------------------
// Main Service Functions
//----------------------------------------------------------------------------------------------------

// sendSignalMessage receives a byte slice from NATS, unmarshals it, and sends it to the
// signal-cli's HTTP API as a JSON-RPC payload.
func sendSignalMessage(data []byte) {
	var natsMessage ResponseNatsMessage
	if err := json.Unmarshal(data, &natsMessage); err != nil {
		log.Printf("Error decoding JSON message from NATS: %v", err)
		return
	}

	// Prepare the nested 'params' structure from the NATS payload.
	params := SignalResponseParams{
		Recipient:  natsMessage.Recipient,
		Message:    natsMessage.Message,
		Attachment: natsMessage.Attachment,
		Account:    natsMessage.Account,
	}

	// Construct the full JSON-RPC payload to be sent to signal-cli.
	signalMessage := ResponseSignalMessage{
		JSONRPC: "2.0",
		Method:  "send",
		Params:  params,
		ID:      1,
	}

	payloadBytes, err := json.Marshal(signalMessage)
	if err != nil {
		log.Printf("Error serializing payload for signal-cli: %v", err)
		return
	}

	// Send the message to signal-cli via an HTTP POST request.
	req, err := http.NewRequest("POST", sseURLSend, bytes.NewBuffer(payloadBytes))
	if err != nil {
		log.Printf("Error creating HTTP request: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Error sending message to signal-cli: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("Failed to send message from account '%s'. Status code: %d", natsMessage.Account, resp.StatusCode)
	} else {
		log.Printf("Message sent successfully from account '%s' to recipients %v.", natsMessage.Account, natsMessage.Recipient)
	}
}

// sendSignalMessageService is a long-running goroutine that subscribes to a NATS topic
// and processes outgoing messages to be sent to Signal.
func sendSignalMessageService() {
	nc, err := nats.Connect(natsServer)
	if err != nil {
		log.Fatalf("Error connecting to NATS: %v", err)
	}
	defer nc.Close()

	log.Printf("Connected to NATS at: %s", natsServer)

	_, err = nc.Subscribe(natsSubjectOut, func(msg *nats.Msg) {
		log.Printf("Message received on topic '%s': %s", msg.Subject, string(msg.Data))
		// For each message, process it in a separate goroutine.
		go sendSignalMessage(msg.Data)
	})
	if err != nil {
		log.Fatalf("Error subscribing to topic '%s': %v", natsSubjectOut, err)
	}

	log.Printf("Successfully subscribed to topic '%s'. Waiting for messages...", natsSubjectOut)
	select {}
}

// receiveSignalMessageService is a long-running goroutine that connects to the signal-cli's
// SSE stream and publishes received messages to a NATS topic.
func receiveSignalMessageService() {
	// Get the local hostname for logging and message identification.
	hostName, err := os.Hostname()
	if err != nil {
		log.Fatalf("Error getting hostname: %v", err)
	}

	// Log service configurations at startup.
	log.Printf("Receive with configurations applied:")
	log.Printf("  SSE URL: %s", sseURLReceive)
	log.Printf("  NATS Server: %s", natsServer)
	log.Printf("  NATS Subject: %s", natsSubjectIn)
	log.Printf("  Server Name: %s", serverName)
	log.Printf("  Local hostname: %s", hostName)

	// Initialize NATS client and establish a connection.
	nc, err := nats.Connect(natsServer)
	if err != nil {
		log.Fatalf("Error connecting to NATS: %v", err)
	}
	defer nc.Close()

	// Initialize HTTP client and make a GET request to the SSE endpoint.
	client := &http.Client{}
	req, err := http.NewRequest("GET", sseURLReceive, nil)
	if err != nil {
		log.Fatalf("Error creating SSE request: %v", err)
	}

	req.Header.Set("Accept", "text/event-stream")
	resp, err := client.Do(req)
	if err != nil {
		log.Fatalf("Error connecting to SSE endpoint: %v", err)
	}
	defer resp.Body.Close()

	log.Printf("Connected to SSE endpoint: %s", sseURLReceive)

	// Read the SSE stream line by line.
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || !strings.HasPrefix(line, "data:") {
			continue
		}

		// Extract JSON data from the "data:" prefix.
		dataStr := strings.TrimPrefix(line, "data:")

		var eventData EventData
		if err := json.Unmarshal([]byte(dataStr), &eventData); err != nil {
			log.Printf("Error validating event data: %v", err)
			continue
		}

		// Log key message details for easy tracking.
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

		// Create the NATS payload with a unique ID and server timestamp.
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

		// Publish the new payload to the NATS inbound topic.
		if err := nc.Publish(natsSubjectIn, eventDataNatsJSON); err != nil {
			log.Printf("Error publishing message to NATS: %v", err)
		} else {
			log.Printf("Message published to NATS topic '%s': %s", natsSubjectIn, string(eventDataNatsJSON))
		}
	}

	if err := scanner.Err(); err != nil {
		log.Fatalf("Error reading SSE stream: %v", err)
	}
}

func main() {
	// Start the message receiving service in a goroutine.
	go receiveSignalMessageService()

	// Start the message sending service in a goroutine.
	go sendSignalMessageService()

	// Keep the main goroutine running indefinitely.
	select {}
}
