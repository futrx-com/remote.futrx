package filechat

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sort"
	"strconv"

	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
)

type pendingTranscriptItem struct {
	event       servicechat.Event
	itemKey     string
	startOffset int64
	endOffset   int64
	turnOrdinal int64
	displaySeq  int64
}

// transcriptProjectionWriter owns materialized items and replacement snapshots
// within one index checkpoint. The event writer owns the surrounding transaction.
type transcriptProjectionWriter struct {
	ctx     context.Context
	tx      *sql.Tx
	chatID  servicechat.ID
	pending map[string]pendingTranscriptItem
}

func newTranscriptProjectionWriter(
	ctx context.Context,
	tx *sql.Tx,
	chatID servicechat.ID,
) *transcriptProjectionWriter {
	return &transcriptProjectionWriter{
		ctx:     ctx,
		tx:      tx,
		chatID:  chatID,
		pending: make(map[string]pendingTranscriptItem),
	}
}

func (writer *transcriptProjectionWriter) indexItem(
	event servicechat.Event,
	eventOrdinal int64,
	turnOrdinal int64,
	startOffset int64,
	endOffset int64,
) error {
	itemKey, mode, visible := transcriptItemIdentity(event, eventOrdinal)
	if !visible {
		return nil
	}
	if mode == transcriptItemReplace {
		key := strconv.FormatInt(turnOrdinal, 10) + "\x00" + itemKey
		displaySeq := event.Seq
		if pending, exists := writer.pending[key]; exists {
			displaySeq = pending.displaySeq
		}
		writer.pending[key] = pendingTranscriptItem{
			event:       event,
			itemKey:     itemKey,
			startOffset: startOffset,
			endOffset:   endOffset,
			turnOrdinal: turnOrdinal,
			displaySeq:  displaySeq,
		}
		return nil
	}
	return writer.persistTranscriptItem(
		event, itemKey, mode, startOffset, endOffset, turnOrdinal, event.Seq,
	)
}

func (writer *transcriptProjectionWriter) flush() error {
	keys := make([]string, 0, len(writer.pending))
	for key := range writer.pending {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		item := writer.pending[key]
		if err := writer.persistTranscriptItem(
			item.event,
			item.itemKey,
			transcriptItemReplace,
			item.startOffset,
			item.endOffset,
			item.turnOrdinal,
			item.displaySeq,
		); err != nil {
			return err
		}
	}
	return nil
}

func (writer *transcriptProjectionWriter) persistTranscriptItem(
	event servicechat.Event,
	itemKey string,
	mode transcriptItemMode,
	startOffset int64,
	endOffset int64,
	turnOrdinal int64,
	displaySeq int64,
) error {
	existingStart, existingPayload, found, err := readTranscriptItem(
		writer.ctx,
		writer.tx,
		writer.chatID,
		turnOrdinal,
		itemKey,
		mode == transcriptItemAppend,
	)
	if err != nil {
		return err
	}

	projected, refs := compactTranscriptEvent(
		writer.chatID, event, itemKey, startOffset, endOffset-startOffset, turnOrdinal,
	)
	projected.Seq = displaySeq
	payload := []servicechat.Event{projected}
	startSeq := projected.Seq
	if found {
		startSeq = existingStart
		switch mode {
		case transcriptItemReplace:
			// Replacement items render at their first occurrence even though
			// their payload is the latest state snapshot.
			projected.Seq = existingStart
			payload[0] = projected
			if err := deleteTranscriptContentRefs(
				writer.ctx, writer.tx, writer.chatID, turnOrdinal, itemKey,
			); err != nil {
				return err
			}
		case transcriptItemAppend:
			if err := json.Unmarshal(existingPayload, &payload); err != nil {
				return err
			}
			payload = append(payload, projected)
		}
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if _, err := writer.tx.ExecContext(writer.ctx, `
		INSERT INTO chat_transcript_items
			(chat_id, turn_ordinal, item_key, start_seq, end_seq,
			 payload_json, payload_bytes)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(chat_id, turn_ordinal, item_key) DO UPDATE SET
			end_seq = excluded.end_seq,
			payload_json = excluded.payload_json,
			payload_bytes = excluded.payload_bytes`,
		writer.chatID,
		turnOrdinal,
		itemKey,
		startSeq,
		event.Seq,
		payloadJSON,
		len(payloadJSON),
	); err != nil {
		return err
	}
	for _, ref := range refs {
		if _, err := writer.tx.ExecContext(writer.ctx, `
			INSERT INTO chat_transcript_content_refs
				(chat_id, content_id, turn_ordinal, item_key, field_kind,
				 field_key, source_offset, source_length, content_bytes)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(chat_id, content_id) DO UPDATE SET
				source_offset = excluded.source_offset,
				source_length = excluded.source_length,
				content_bytes = excluded.content_bytes`,
			writer.chatID,
			ref.id,
			turnOrdinal,
			itemKey,
			ref.fieldKind,
			ref.fieldKey,
			ref.sourceOffset,
			ref.sourceLength,
			ref.contentBytes,
		); err != nil {
			return err
		}
	}
	return nil
}

func readTranscriptItem(
	ctx context.Context,
	tx *sql.Tx,
	chatID servicechat.ID,
	turnOrdinal int64,
	itemKey string,
	includePayload bool,
) (int64, []byte, bool, error) {
	var startSeq int64
	var payload []byte
	if !includePayload {
		err := tx.QueryRowContext(ctx, `
			SELECT start_seq
			FROM chat_transcript_items
			WHERE chat_id = ? AND turn_ordinal = ? AND item_key = ?`,
			chatID, turnOrdinal, itemKey,
		).Scan(&startSeq)
		if errors.Is(err, sql.ErrNoRows) {
			return 0, nil, false, nil
		}
		return startSeq, nil, err == nil, err
	}
	err := tx.QueryRowContext(ctx, `
		SELECT start_seq, payload_json
		FROM chat_transcript_items
		WHERE chat_id = ? AND turn_ordinal = ? AND item_key = ?`,
		chatID, turnOrdinal, itemKey,
	).Scan(&startSeq, &payload)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil, false, nil
	}
	return startSeq, payload, err == nil, err
}

func deleteTranscriptContentRefs(
	ctx context.Context,
	tx *sql.Tx,
	chatID servicechat.ID,
	turnOrdinal int64,
	itemKey string,
) error {
	_, err := tx.ExecContext(ctx, `
		DELETE FROM chat_transcript_content_refs
		WHERE chat_id = ? AND turn_ordinal = ? AND item_key = ?`,
		chatID, turnOrdinal, itemKey,
	)
	return err
}
