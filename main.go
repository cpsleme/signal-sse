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

// responseNatsMessage represents the JSON payload received from the NATS 'outbound' topic.
type ResponseNatsMessage struct {
	Recipient  string `json:"recipient"`
	Message    string `json:"message"`
	Attachment string `json:"attachment,omitempty"`
	Account    string `json:"account"`
}

// responseSignalMessage represents the full JSON-RPC payload to be sent to signal-cli's API.
type ResponseSignalMessage struct {
	JSONRPC string              `json:"jsonrpc"`
	Method  string              `json:"method"`
	Params  ResponseNatsMessage `json:"params"`
	ID      int                 `json:"id"`
}

//----------------------------------------------------------------------------------------------------
// Service Configuration
//----------------------------------------------------------------------------------------------------

var (
	sseHost         = "localhost"
	ssePort         = 8080
	natsServer      = "nats://localhost:4222"
	natsSubjectIn   = "signal.inbound"  // NATS topic for incoming Signal messages
	natsSubjectOut  = "signal.outbound" // NATS topic for outgoing Signal messages
	sseURLReceive   = fmt.Sprintf("http://%s:%d/api/v1/events", sseHost, ssePort)
	sseURLSend      = fmt.Sprintf("http://%s:%d/api/v1/rpc", sseHost, ssePort)
	serverName      = fmt.Sprintf("%s:%d", sseHost, ssePort)
	jetStreamBucket = "signal-semaphore"
	heartbeatKey    = "active"
)

//----------------------------------------------------------------------------------------------------
// Main Service Functions
//----------------------------------------------------------------------------------------------------

// Get hostname once and store it in the global variable.
func getHostname() string {
	var err error
	var hostName string

	hostName, err = os.Hostname()
	if err != nil {
		log.Fatalf("Fatal Error: Could not get hostname. %v", err)
	}
	return hostName
}

// writeHeartbeatToJetStream writes the hostname to a JetStream key-value store every 5 seconds.
func semaphoreSignalService(js nats.JetStreamContext) {
	kv, err := js.KeyValue(jetStreamBucket)
	if err != nil {
		log.Printf("Key-Value bucket '%s' not found, creating it...", jetStreamBucket)
		kv, err = js.CreateKeyValue(&nats.KeyValueConfig{
			Bucket: jetStreamBucket,
		})
		if err != nil {
			log.Fatalf("Fatal error creating JetStream Key-Value bucket: %v", err)
		}
	}

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	log.Printf("JetStream heartbeat started for bucket '%s' on key '%s'.", jetStreamBucket, heartbeatKey)

	for range ticker.C {
		if _, err := kv.PutString(heartbeatKey, getHostname()); err != nil {
			log.Printf("Error writing heartbeat to JetStream: %v", err)
		} else {
			log.Printf("Heartbeat written to JetStream: key='%s', value='%s'", heartbeatKey, getHostname())
		}
	}
}

// sendSignalMessage is responsible for sending a message to signal-cli's HTTP API.
func sendSignalMessage(data []byte) {
	var natsMessage ResponseNatsMessage
	if err := json.Unmarshal(data, &natsMessage); err != nil {
		log.Printf("Error decoding JSON message from NATS: %v", err)
		return
	}

	signalMessage := ResponseSignalMessage{
		JSONRPC: "2.0",
		Method:  "send",
		Params:  natsMessage,
		ID:      1,
	}

	payloadBytes, err := json.Marshal(signalMessage)
	if err != nil {
		log.Printf("Error serializing payload for signal-cli: %v", err)
		return
	}

	url := fmt.Sprintf("%s", sseURLSend)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(payloadBytes))
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
// and processes outgoing messages to be sent to Signal only if it's the active instance.
func sendSignalMessageService(nc *nats.Conn, js nats.JetStreamContext) {
	log.Printf("Connected to NATS at: %s", natsServer)

	kv, err := js.KeyValue(jetStreamBucket)
	if err != nil {
		log.Fatalf("Fatal error getting Key-Value bucket for send service: %v", err)
	}

	_, err = nc.Subscribe(natsSubjectOut, func(msg *nats.Msg) {

		activeHost, err := kv.Get(heartbeatKey)
		if err != nil {
			log.Printf("Error getting active hostname from JetStream: %v", err)
			return
		}

		if string(activeHost.Value()) != getHostname() {
			log.Printf("Passive instance. Skipping message from topic '%s'. Active host is '%s'.", msg.Subject, activeHost)
			return
		}

		log.Printf("Active instance. Processing message from topic '%s': %s", msg.Subject, string(msg.Data))
		go sendSignalMessage(msg.Data)
	})
	if err != nil {
		log.Fatalf("Error subscribing to topic '%s': %v", natsSubjectOut, err)
	}

	log.Printf("Successfully subscribed to topic '%s'. Waiting for messages...", natsSubjectOut)
	select {}
}

// receiveSignalMessageService is a long-running goroutine that connects to the signal-cli's
// SSE stream and publishes received messages to a NATS topic only if it's the active instance.
func receiveSignalMessageService(nc *nats.Conn, js nats.JetStreamContext) {
	log.Printf("Receive with configurations applied:")
	log.Printf("  SSE URL: %s", sseURLReceive)
	log.Printf("  NATS Server: %s", natsServer)
	log.Printf("  NATS Subject: %s", natsSubjectIn)
	log.Printf("  Server Name: %s", serverName)
	log.Printf("  Local hostname: %s", getHostname())

	kv, err := js.KeyValue(jetStreamBucket)
	if err != nil {
		log.Fatalf("Fatal error getting Key-Value bucket for receive service: %v", err)
	}

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

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		activeHost, err := kv.Get(heartbeatKey)
		if err != nil {
			log.Printf("Error getting active hostname from JetStream: %v", err)
			continue
		}

		if string(activeHost.Value()) != getHostname() {
			log.Printf("Passive instance. Skipping message from SSE stream. Active host is '%s'.", activeHost)
			continue
		}

		line := scanner.Text()
		if line == "" || !strings.HasPrefix(line, "data:") {
			continue
		}

		dataStr := strings.TrimPrefix(line, "data:")

		var eventData EventData
		if err := json.Unmarshal([]byte(dataStr), &eventData); err != nil {
			log.Printf("Error validating event data: %v", err)
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

		log.Printf("Message received:")
		log.Printf("  Account: %s", account)
		log.Printf("  From: %s (%s)", sourceNumber, sourceName)
		log.Printf("  Content: %s", messageContent)
		log.Printf("  TimestampSignal: %d", timestamp)

		eventDataNats := EventDataNats{
			ID:              uuid.NewString(),
			Server:          getHostname(),
			TimestampServer: time.Now().UnixMilli(),
			Message:         &eventData,
		}
		eventDataNatsJSON, err := json.Marshal(eventDataNats)
		if err != nil {
			log.Printf("Error serializing EventDataNats to JSON: %v", err)
			continue
		}

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
	var err error

	// Centralized NATS connection
	nc, err := nats.Connect(natsServer)
	if err != nil {
		log.Fatalf("Fatal Error: Could not connect to NATS. %v", err)
	}
	defer nc.Close()

	// Centralized JetStream context
	js, err := nc.JetStream()
	if err != nil {
		log.Fatalf("Fatal Error: Could not create JetStream context. %v", err)
	}

	// Init semaphore service
	go semaphoreSignalService(js)
	time.Sleep(3 * time.Second)

	// Init receiving messages
	go receiveSignalMessageService(nc, js)

	// Init sending messages
	go sendSignalMessageService(nc, js)

	select {}
}
