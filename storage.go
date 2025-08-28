package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"time"

	_ "github.com/go-sql-driver/mysql" // Import the MySQL driver
	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
)

// designed to store details from both incoming and outgoing Signal messages.
type HistoryRecord struct {
	ID                string    // Unique ID for this history record
	EventType         string    // "inbound" or "outbound"
	OriginalMessageID string    // Original message ID from Signal/NATS
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

// HistoryDB provides database operations for tb_history and tb_attachments.
type HistoryDB struct {
	db *sql.DB
}

// ConnectHistoryDB initializes and returns a new HistoryDB instance for MySQL.
// It takes the MySQL DSN (Data Source Name) as input.
func ConnectHistoryDB(mysqlDSN string) (*HistoryDB, error) {
	db, err := sql.Open("mysql", mysqlDSN)
	if err != nil {
		return nil, fmt.Errorf("failed to open MySQL database: %w", err)
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(5 * time.Minute)

	if err = db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to connect to MySQL database: %w", err)
	}

	log.Printf("Successfully connected to MySQL database using DSN.")
	return &HistoryDB{db: db}, nil
}

// Close the database connection.
func (h *HistoryDB) Close() error {
	return h.db.Close()
}

// CreateTables creates the tb_history and tb_attachments tables if they don't already exist.
func (h *HistoryDB) createTables() error {
	const createHistoryTableSQL = `
	CREATE TABLE IF NOT EXISTS tb_history (
		id VARCHAR(255) PRIMARY KEY,
		event_type VARCHAR(50) NOT NULL,
		original_message_id VARCHAR(255),
		account VARCHAR(255) NOT NULL,
		sender_number VARCHAR(255),
		sender_name VARCHAR(255),
		recipient VARCHAR(255),
		message_content TEXT,
		attachments_exist BOOLEAN,
		timestamp_service BIGINT NOT NULL,
		timestamp_signal BIGINT,
		raw_payload JSON NOT NULL,
		processed_by_server VARCHAR(255) NOT NULL,
		logged_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);`

	_, err := h.db.Exec(createHistoryTableSQL)
	if err != nil {
		return fmt.Errorf("failed to create tb_history table: %w", err)
	}
	log.Println("tb_history table ensured.")

	const createHistoryAttachmentsTableSQL = `
	CREATE TABLE IF NOT EXISTS tb_history_attachments (
		id VARCHAR(255) PRIMARY KEY,
		history_id VARCHAR(255) NOT NULL,
		content_type VARCHAR(255),
		filename VARCHAR(255),
		size INT,
		width INT,
		height INT,
		caption TEXT,
		upload_timestamp BIGINT,
		logged_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (history_id) REFERENCES tb_history(id) ON DELETE CASCADE
	);`

	_, err = h.db.Exec(createHistoryAttachmentsTableSQL)
	if err != nil {
		return fmt.Errorf("failed to create tb_history_attachments table: %w", err)
	}
	log.Println("tb_history_attachments table ensured.")

	// Create Indexes
	const createIndexesSQL = `
	CREATE INDEX IF NOT EXISTS idx_history_id ON tb_history (id)
	CREATE INDEX IF NOT EXISTS idx_history_sender_number ON tb_history (sender_number);
	CREATE INDEX IF NOT EXISTS idx_history_recipient ON tb_history (recipient);
	CREATE INDEX IF NOT EXISTS idx_history_timestamp_service ON tb_history (timestamp_service);
	CREATE INDEX IF NOT EXISTS idx_history_attachments_id ON tb_history_attachments (id)
	CREATE INDEX IF NOT EXISTS idx_history_attachments_history_id ON tb_history_attachments (history_id);
	`

	_, err = h.db.Exec(createIndexesSQL)
	if err != nil {
		return fmt.Errorf("failed to create indexes: %w", err)
	}
	log.Println("Database indexes ensured.")

	return nil
}

// InsertInboundMessage inserts an incoming message payload into tb_history
// and any associated attachments into tb_attachments.
// It requires the server's hostname to log which server processed the message.
func (h *HistoryDB) insertInboundMessage(payload *InboundNatsMessagePayload, processedByServer string) error {
	rawPayloadJSON, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal inbound payload to JSON: %w", err)
	}

	historyID := payload.ID

	var messageContent string
	if payload.EventData != nil &&
		payload.EventData.Envelope.SyncMessage.SentMessage != nil &&
		payload.EventData.Envelope.SyncMessage.SentMessage.Message != nil {
		messageContent = *payload.EventData.Envelope.SyncMessage.SentMessage.Message
	} else {
		messageContent = "<no message content>"
	}

	var attachmentsExist bool
	var attachments []Attachment
	if payload.EventData != nil &&
		payload.EventData.Envelope.SyncMessage.SentMessage != nil &&
		payload.EventData.Envelope.SyncMessage.SentMessage.Attachments != nil &&
		len(*payload.EventData.Envelope.SyncMessage.SentMessage.Attachments) > 0 {
		attachmentsExist = true
		attachments = *payload.EventData.Envelope.SyncMessage.SentMessage.Attachments
	}

	record := HistoryRecord{
		ID:                historyID,
		EventType:         "inbound",
		OriginalMessageID: fmt.Sprintf("%d", payload.EventData.Envelope.Timestamp),
		Account:           payload.EventData.Account,
		SenderNumber:      payload.EventData.Envelope.SourceNumber,
		SenderName:        payload.EventData.Envelope.SourceName,
		MessageContent:    messageContent,
		AttachmentsExist:  attachmentsExist,
		TimestampService:  payload.TimestampServer,
		TimestampSignal:   payload.EventData.Envelope.Timestamp,
		RawPayload:        string(rawPayloadJSON),
		ProcessedByServer: processedByServer,
		LoggedAt:          time.Now().UTC(),
	}

	tx, err := h.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction for inbound message: %w", err)
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			panic(r)
		} else if err != nil {
			tx.Rollback()
		} else {
			err = tx.Commit()
		}
	}()

	if err = h.insertHistoryRecord(tx, record); err != nil {
		return fmt.Errorf("failed to insert inbound history record: %w", err)
	}

	for _, att := range attachments {
		attachmentRecord := HistoryAttachmentRecord{
			ID:              uuid.NewString(),
			HistoryID:       historyID,
			ContentType:     att.ContentType,
			Filename:        valueOrDefault(att.Filename),
			Size:            att.Size,
			Width:           att.Width,
			Height:          att.Height,
			Caption:         valueOrDefault(att.Caption),
			UploadTimestamp: att.UploadTimestamp,
		}
		if err = h.insertHistoryAttachmentRecord(tx, attachmentRecord); err != nil {
			return fmt.Errorf("failed to insert inbound attachment record: %w", err)
		}
	}

	log.Printf("Inbound message '%s' (%s) and %d attachments inserted successfully.", record.ID, record.EventType, len(attachments))
	return err
}

// InsertOutboundMessage inserts an outgoing message payload into tb_history
// and any associated attachments into tb_attachments.
// It requires the server's hostname to log which server processed the message.
func (h *HistoryDB) insertOutboundMessage(payload *SignalOutboundMessage, serviceTimestamp int64, processedByServer string) error {
	rawPayloadJSON, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal outbound payload to JSON: %w", err)
	}

	historyID := uuid.NewString()

	var attachmentsExist bool
	if payload.Attachment != "" {
		attachmentsExist = true
	}

	record := HistoryRecord{
		ID:                historyID,
		EventType:         "outbound",
		OriginalMessageID: "N/A",
		Account:           payload.Account,
		Recipient:         payload.Recipient,
		MessageContent:    payload.Message,
		AttachmentsExist:  attachmentsExist,
		TimestampService:  serviceTimestamp,
		TimestampSignal:   0,
		RawPayload:        string(rawPayloadJSON),
		ProcessedByServer: processedByServer,
		LoggedAt:          time.Now().UTC(),
	}

	tx, err := h.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction for outbound message: %w", err)
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			panic(r)
		} else if err != nil {
			tx.Rollback()
		} else {
			err = tx.Commit()
		}
	}()

	if err = h.insertHistoryRecord(tx, record); err != nil {
		return fmt.Errorf("failed to insert outbound history record: %w", err)
	}

	if attachmentsExist {
		attachmentRecord := HistoryAttachmentRecord{
			ID:              uuid.NewString(),
			HistoryID:       historyID,
			ContentType:     "unknown",
			Filename:        payload.Attachment,
			Size:            0,
			Width:           0,
			Height:          0,
			Caption:         "",
			UploadTimestamp: int(serviceTimestamp / 1000),
			LoggedAt:        time.Now().UTC(),
		}
		if err = h.insertHistoryAttachmentRecord(tx, attachmentRecord); err != nil {
			return fmt.Errorf("failed to insert outbound attachment record: %w", err)
		}
		log.Printf("Outbound message '%s' (%s) and 1 attachment inserted successfully.", record.ID, record.EventType)
	} else {
		log.Printf("Outbound message '%s' (%s) inserted successfully.", record.ID, record.EventType)
	}

	return err
}

// insertHistoryRecord is a private helper to execute the SQL INSERT statement for tb_history within a transaction.
func (h *HistoryDB) insertHistoryRecord(tx *sql.Tx, record HistoryRecord) error {
	const insertSQL = `
	INSERT INTO tb_history (
		id, event_type, original_message_id, account, sender_number, sender_name,
		recipient, message_content, attachments_exist, timestamp_service,
		timestamp_signal, raw_payload, processed_by_server, logged_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);`

	_, err := tx.Exec(insertSQL,
		record.ID,
		record.EventType,
		record.OriginalMessageID,
		record.Account,
		record.SenderNumber,
		record.SenderName,
		record.Recipient,
		record.MessageContent,
		record.AttachmentsExist,
		record.TimestampService,
		record.TimestampSignal,
		record.RawPayload,
		record.ProcessedByServer,
		record.LoggedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to insert history record into tb_history: %w", err)
	}
	return nil
}

// insertHistoryAttachmentRecord is a private helper to execute the SQL INSERT statement for tb_attachments within a transaction.
func (h *HistoryDB) insertHistoryAttachmentRecord(tx *sql.Tx, record HistoryAttachmentRecord) error {
	const insertSQL = `
	INSERT INTO tb_history_attachments (
		id, history_id, content_type, filename, size, width, height, caption, upload_timestamp, logged_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?);`

	_, err := tx.Exec(insertSQL,
		record.ID,
		record.HistoryID,
		record.ContentType,
		record.Filename,
		record.Size,
		record.Width,
		record.Height,
		record.Caption,
		record.UploadTimestamp,
		record.LoggedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to insert attachment record into tb_attachments: %w", err)
	}
	return nil
}

// startHistoryNatsSubscribers configures and starts NATS subscribers dedicated to logging messages to history.
func startHistoryNatsSubscribers(ctx context.Context, nc *nats.Conn, cfg *Config, historyDB *HistoryDB) {
	log.Println("Starting NATS history subscribers...")

	// Subscriber for inbound messages
	// Subscribes to the standard NATS topic where `receiveSignalMessageService` (in another service) publishes.
	// Using QueueSubscribe for scalability among multiple history logger instances.
	inboundSub, err := nc.QueueSubscribe(cfg.NatsSubjectIn, "history-inbound-group", func(msg *nats.Msg) {
		log.Printf("Received INBOUND NATS message for history on topic '%s'.", msg.Subject)
		var inboundPayload InboundNatsMessagePayload
		if err := json.Unmarshal(msg.Data, &inboundPayload); err != nil {
			log.Printf("Error decoding INBOUND NATS payload for history: %v", err)
			return
		}

		if err := historyDB.insertInboundMessage(&inboundPayload, getHostname()); err != nil {
			log.Printf("Error logging INBOUND message to history: %v", err)
		}
	})
	if err != nil {
		log.Fatalf("Failed to subscribe to topic '%s' for inbound history: %v", cfg.NatsSubjectIn, err)
	}
	log.Printf("Successfully subscribed to topic '%s' (group 'history-inbound-group') for inbound history logging.", cfg.NatsSubjectIn)

	// Subscriber for outbound messages
	// Subscribes to the standard NATS topic where outbound messages are published (e.g., before going to Signal CLI API).
	// Using QueueSubscribe for scalability among multiple history logger instances.
	outboundSub, err := nc.QueueSubscribe(cfg.NatsSubjectOut, "history-outbound-group", func(msg *nats.Msg) {
		log.Printf("Received OUTBOUND NATS message for history on topic '%s'.", msg.Subject)
		var outboundMessage SignalOutboundMessage
		if err := json.Unmarshal(msg.Data, &outboundMessage); err != nil {
			log.Printf("Error decoding OUTBOUND NATS payload for history: %v", err)
			return
		}

		// The service timestamp for the outbound message is generated here, at the time of logging.
		serviceTimestamp := time.Now().UnixMilli()
		if err := historyDB.insertOutboundMessage(&outboundMessage, serviceTimestamp, getHostname()); err != nil {
			log.Printf("Error logging OUTBOUND message to history: %v", err)
		}
	})
	if err != nil {
		log.Fatalf("Failed to subscribe to topic '%s' for outbound history: %v", cfg.NatsSubjectOut, err)
	}
	log.Printf("Successfully subscribed to topic '%s' (group 'history-outbound-group') for outbound history logging.", cfg.NatsSubjectOut)

	<-ctx.Done() // Wait for the context to be cancelled to shut down subscribers
	log.Println("Context cancelled, shutting down history subscribers.")

	if err := inboundSub.Unsubscribe(); err != nil {
		log.Printf("Error unsubscribing from INBOUND topic: %v", err)
	}
	if err := outboundSub.Unsubscribe(); err != nil {
		log.Printf("Error unsubscribing from OUTBOUND topic: %v", err)
	}
}

// Entry point
func startStorage(ctx context.Context, nc *nats.Conn, cfg *Config) {

	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)

	log.Printf("Attempting to connect to MySQL database...")

	historyDB, err := ConnectHistoryDB(cfg.MySQLDSN)
	if err != nil {
		log.Fatalf("Could not connect to history database: %v", err)
	}
	defer func() {
		if closeErr := historyDB.Close(); closeErr != nil {
			log.Printf("Error closing history database: %v", closeErr)
		}
	}()

	if err := historyDB.createTables(); err != nil {
		log.Fatalf("Could not create history tables: %v", err)
	}

	// Start only the history subscribers. This function will block until ctx is cancelled.
	startHistoryNatsSubscribers(ctx, nc, cfg, historyDB)

	log.Println("NATS history service shut down. Exiting application.")
}
