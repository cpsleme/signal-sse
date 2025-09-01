package main

import (
	"time"
)

//----------------------------------------------------------------------------------------------------
// Structs for Incoming Payloads (Signal CLI SSE -> NATS)
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

// SentMessageIn represents the sent message structure within a SyncMessage.
type SentMessageIn struct {
	Destination       string        `json:"destination"`
	DestinationNumber string        `json:"destinationNumber"`
	DestinationUUID   string        `json:"destinationUuid"`
	Timestamp         int64         `json:"timestamp"`
	Message           *string       `json:"message"`
	ExpiresInSeconds  int           `json:"expiresInSeconds"`
	ViewOnce          bool          `json:"viewOnce"`
	Attachments       *[]Attachment `json:"attachments"`
}

// SyncMessageIn represents the sync message structure received from Signal.
type SyncMessageIn struct {
	SentMessage *SentMessageIn `json:"sentMessage"`
}

// EnvelopeIn represents the core message envelope containing sender and message details.
type EnvelopeIn struct {
	Source                   string        `json:"source"`
	SourceNumber             string        `json:"sourceNumber"`
	SourceUUID               string        `json:"sourceUuid"`
	SourceName               string        `json:"sourceName"`
	SourceDevice             int           `json:"sourceDevice"`
	Timestamp                int64         `json:"timestamp"`
	ServerReceivedTimestamp  int64         `json:"serverReceivedTimestamp"`
	ServerDeliveredTimestamp int64         `json:"serverDeliveredTimestamp"`
	SyncMessage              SyncMessageIn `json:"syncMessage"`
}

// EventDataIn represents the top-level event data received from the Signal SSE stream.
type EventDataIn struct {
	Envelope EnvelopeIn `json:"envelope"`
	Account  string     `json:"account"`
}

// InboundNatsMessagePayload represents the payload structure for messages published to the NATS 'inbound' topic.
// It wraps the raw Signal event data with a unique ID and server-side timestamp.
type InboundNatsMessagePayload struct {
	ID              string       `json:"id"`
	Server          string       `json:"server"`
	TimestampServer int64        `json:"timestamp"`
	EventData       *EventDataIn `json:"eventData"`
}

//----------------------------------------------------------------------------------------------------
// Structs for Outgoing Payloads (NATS -> Signal CLI RPC)
//----------------------------------------------------------------------------------------------------

// SignalOutboundMessage represents the JSON payload received from the NATS 'outbound' topic.
type SignalOutboundMessage struct {
	Recipient  string `json:"recipient"`
	Message    string `json:"message"`
	Attachment string `json:"attachment,omitempty"`
	Account    string `json:"account"`
}

// SignalRPCRequest represents the full JSON-RPC payload to be sent to signal-cli's API.
type SignalRPCRequest struct {
	JSONRPC string                `json:"jsonrpc"`
	Method  string                `json:"method"`
	Params  SignalOutboundMessage `json:"params"`
	ID      int                   `json:"id"`
}

// OutboundNatsMessagePayload represents the payload structure for messages received from NATS 'outbound' topic.
// It wraps the SignalOutboundMessage with a unique ID and server-side timestamp for internal tracking.
type OutboundNatsMessagePayload struct {
	ID              string                 `json:"id"`
	Server          string                 `json:"server"`
	TimestampServer int64                  `json:"timestamp"`
	Message         *SignalOutboundMessage `json:"message"`
}

//----------------------------------------------------------------------------------------------------
// Structs for History Database
//----------------------------------------------------------------------------------------------------

// designed to store details from both incoming and outgoing Signal messages.
type HistoryRecord struct {
	ID                string    // Unique ID for this history record
	EventType         string    // "inbound" or "outbound"
	Account           string    // Signal account involved
	SenderNumber      string    // Sender's phone number (for inbound)
	SenderName        string    // Sender's name (for inbound)
	Recipient         string    // Recipient's phone number (for outbound)
	MessageContent    string    // The actual text message content
	AttachmentsExist  bool      // True if attachments were present
	TimestampService  int64     // When our service processed/logged it (Unix Millis)
	TimestampSignal   int64     // Original timestamp from Signal (Unix Millis)
	RawPayload        string    // Raw JSON payload of the original message
	ProcessedByServer string    // The server that processed this message
	LoggedAt          time.Time // When this record was inserted into history (UTC)
}

// AttachmentRecord represents a single entry in the tb_attachments table.
type HistoryAttachmentRecord struct {
	ID              string    // Unique ID for this attachment record
	HistoryID       string    // Foreign key to tb_history
	ContentType     string    // e.g., "image/jpeg"
	Filename        string    // Original filename
	Size            int       // Size in bytes
	Width           int       // Image width (if applicable)
	Height          int       // Image height (if applicable)
	Caption         string    // Caption for the attachment
	UploadTimestamp int       // Original upload timestamp from Signal
	LoggedAt        time.Time // When this record was inserted into history (UTC)
}
