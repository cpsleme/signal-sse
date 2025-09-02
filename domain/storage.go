package domain

import "time"

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
