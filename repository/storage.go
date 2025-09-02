package repository

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"time"

	"signal-sse/domain"
	"signal-sse/infra"
	util "signal-sse/utils"

	"github.com/google/uuid"
)

// InsertInboundMessage inserts an incoming message payload into tb_history
// and any associated attachments into tb_attachments.
// It requires the server's hostname to log which server processed the message.
func InsertInboundMessage(h *infra.HistoryDB, payload *domain.InboundNatsMessagePayload, processedByServer string) error {
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
	var attachments []domain.Attachment
	if payload.EventData != nil &&
		payload.EventData.Envelope.SyncMessage.SentMessage != nil &&
		payload.EventData.Envelope.SyncMessage.SentMessage.Attachments != nil &&
		len(*payload.EventData.Envelope.SyncMessage.SentMessage.Attachments) > 0 {
		attachmentsExist = true
		attachments = *payload.EventData.Envelope.SyncMessage.SentMessage.Attachments
	}

	record := domain.HistoryRecord{
		ID:                historyID,
		EventType:         "inbound",
		Account:           payload.EventData.Account,
		SenderNumber:      payload.EventData.Envelope.SourceNumber,
		SenderName:        payload.EventData.Envelope.SourceName,
		MessageContent:    messageContent,
		AttachmentsExist:  attachmentsExist,
		TimestampService:  payload.TimestampServer,
		TimestampSignal:   payload.EventData.Envelope.Timestamp,
		RawPayload:        string(rawPayloadJSON),
		ProcessedByServer: processedByServer,
		LoggedAt:          time.Now(),
	}

	tx, err := h.DB.Begin()
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

	if err = insertHistoryRecord(tx, record); err != nil {
		return fmt.Errorf("failed to insert inbound history record: %w", err)
	}

	for _, att := range attachments {
		attachmentRecord := domain.HistoryAttachmentRecord{
			ID:              uuid.NewString(),
			HistoryID:       historyID,
			ContentType:     att.ContentType,
			Filename:        util.ValueOrDefault(att.Filename),
			Size:            att.Size,
			Width:           att.Width,
			Height:          att.Height,
			Caption:         util.ValueOrDefault(att.Caption),
			UploadTimestamp: att.UploadTimestamp,
			LoggedAt:        time.Now(),
		}
		if err = insertHistoryAttachmentRecord(tx, attachmentRecord); err != nil {
			return fmt.Errorf("failed to insert inbound attachment record: %w", err)
		}
	}

	log.Printf("Inbound message '%s' (%s) and %d attachments inserted successfully.", record.ID, record.EventType, len(attachments))
	return err
}

// InsertOutboundMessage inserts an outgoing message payload into tb_history
// and any associated attachments into tb_attachments.
// It requires the server's hostname to log which server processed the message.
func InsertOutboundMessage(h *infra.HistoryDB, payload *domain.SignalOutboundMessage, serviceTimestamp int64, processedByServer string) error {
	rawPayloadJSON, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal outbound payload to JSON: %w", err)
	}

	historyID := uuid.NewString() + "-" + strconv.FormatInt(serviceTimestamp, 10)

	var attachmentsExist bool
	if payload.Attachment != "" {
		attachmentsExist = true
	}

	record := domain.HistoryRecord{
		ID:                historyID,
		EventType:         "outbound",
		Account:           payload.Account,
		Recipient:         payload.Recipient,
		MessageContent:    payload.Message,
		AttachmentsExist:  attachmentsExist,
		TimestampService:  serviceTimestamp,
		TimestampSignal:   0,
		RawPayload:        string(rawPayloadJSON),
		ProcessedByServer: processedByServer,
		LoggedAt:          time.Now(),
	}

	tx, err := h.DB.Begin()
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

	if err = insertHistoryRecord(tx, record); err != nil {
		return fmt.Errorf("failed to insert outbound history record: %w", err)
	}

	if attachmentsExist {
		attachmentRecord := domain.HistoryAttachmentRecord{
			ID:              uuid.NewString(),
			HistoryID:       historyID,
			ContentType:     "unknown",
			Filename:        payload.Attachment,
			Size:            0,
			Width:           0,
			Height:          0,
			Caption:         "",
			UploadTimestamp: int(serviceTimestamp / 1000),
			LoggedAt:        time.Now(),
		}
		if err = insertHistoryAttachmentRecord(tx, attachmentRecord); err != nil {
			return fmt.Errorf("failed to insert outbound attachment record: %w", err)
		}
		log.Printf("Outbound message '%s' (%s) and 1 attachment inserted successfully.", record.ID, record.EventType)
	} else {
		log.Printf("Outbound message '%s' (%s) inserted successfully.", record.ID, record.EventType)
	}

	return err
}

// insertHistoryRecord is a private helper to execute the SQL INSERT statement for tb_history within a transaction.
func insertHistoryRecord(tx *sql.Tx, record domain.HistoryRecord) error {
	const insertSQL = `
	INSERT INTO tb_history (
		id, event_type, account, sender_number, sender_name,
		recipient, message_content, attachments_exist, timestamp_service,
		timestamp_signal, raw_payload, processed_by_server, logged_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);`

	_, err := tx.Exec(insertSQL,
		record.ID,
		record.EventType,
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
func insertHistoryAttachmentRecord(tx *sql.Tx, record domain.HistoryAttachmentRecord) error {
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
