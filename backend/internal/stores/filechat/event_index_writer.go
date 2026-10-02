package filechat

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"

	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
)

type chatIndexWriter struct {
	ctx    context.Context
	tx     *sql.Tx
	chatID servicechat.ID
	state  chatIndexState
	turn   indexedTurnState
	// sourceBySeq reads the event stream from chats.sqlite instead of the
	// JSONL file, so byte offsets are meaningless and the offset index is not
	// written. Turns and content refs are then addressed by sequence.
	sourceBySeq  bool
	eventOffsets *sql.Stmt
	insertTurn   *sql.Stmt
	updateTurn   *sql.Stmt
	transcript   *transcriptProjectionWriter
}

func newChatIndexWriter(
	ctx context.Context,
	tx *sql.Tx,
	chatID servicechat.ID,
	state chatIndexState,
	turn indexedTurnState,
) *chatIndexWriter {
	return &chatIndexWriter{
		ctx:        ctx,
		tx:         tx,
		chatID:     chatID,
		state:      state,
		turn:       turn,
		transcript: newTranscriptProjectionWriter(ctx, tx, chatID),
	}
}

func (writer *chatIndexWriter) prepare() error {
	var err error
	if writer.sourceBySeq {
		writer.transcript.seqSource = true
		writer.insertTurn, err = writer.tx.PrepareContext(writer.ctx, `
			INSERT INTO chat_transcript_turns
				(chat_id, turn_ordinal, source_turn_id, has_user,
				 start_seq, end_seq)
			VALUES (?, ?, ?, ?, ?, ?)`)
		if err != nil {
			return err
		}
		writer.updateTurn, err = writer.tx.PrepareContext(writer.ctx, `
			UPDATE chat_transcript_turns
			SET source_turn_id = ?, has_user = ?, end_seq = ?
			WHERE chat_id = ? AND turn_ordinal = ?`)
		if err != nil {
			writer.close()
			return err
		}
		return nil
	}
	writer.eventOffsets, err = writer.tx.PrepareContext(writer.ctx, `
		INSERT INTO chat_event_offsets
			(chat_id, event_ordinal, event_seq, byte_offset, byte_length)
		VALUES (?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	writer.insertTurn, err = writer.tx.PrepareContext(writer.ctx, `
		INSERT INTO chat_transcript_turns
			(chat_id, turn_ordinal, source_turn_id, has_user,
			 start_seq, end_seq, start_offset, end_offset)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		writer.close()
		return err
	}
	writer.updateTurn, err = writer.tx.PrepareContext(writer.ctx, `
		UPDATE chat_transcript_turns
		SET source_turn_id = ?, has_user = ?, end_seq = ?, end_offset = ?
		WHERE chat_id = ? AND turn_ordinal = ?`)
	if err != nil {
		writer.close()
		return err
	}
	return nil
}

func (writer *chatIndexWriter) close() {
	if writer.updateTurn != nil {
		_ = writer.updateTurn.Close()
	}
	if writer.insertTurn != nil {
		_ = writer.insertTurn.Close()
	}
	if writer.eventOffsets != nil {
		_ = writer.eventOffsets.Close()
	}
}

// indexTail scans no farther than observedSize. If another process appends
// concurrently, those bytes are picked up by the next synchronization.
func (writer *chatIndexWriter) indexTail(
	eventsPath string,
	observedSize int64,
) (chatIndexState, error) {
	if observedSize == writer.state.indexedBytes {
		return writer.state, nil
	}
	file, err := os.Open(eventsPath)
	if err != nil {
		return chatIndexState{}, err
	}
	defer file.Close()
	if _, err := file.Seek(writer.state.indexedBytes, io.SeekStart); err != nil {
		return chatIndexState{}, err
	}
	if err := writer.prepare(); err != nil {
		return chatIndexState{}, err
	}
	defer writer.close()

	remaining := observedSize - writer.state.indexedBytes
	reader := bufio.NewReader(io.LimitReader(file, remaining))
	offset := writer.state.indexedBytes
	writer.state.tailComplete = true
	for offset < observedSize {
		if err := writer.ctx.Err(); err != nil {
			return chatIndexState{}, err
		}
		raw, readErr := reader.ReadBytes('\n')
		if len(raw) > 0 {
			startOffset := offset
			offset += int64(len(raw))
			writer.state.tailComplete = raw[len(raw)-1] == '\n'
			writer.state.prefixHash = updateIndexPrefixHash(writer.state.prefixHash, raw)
			if err := writer.indexRecord(raw, startOffset, offset); err != nil {
				return chatIndexState{}, err
			}
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return chatIndexState{}, readErr
		}
	}
	if offset != observedSize {
		return chatIndexState{}, io.ErrUnexpectedEOF
	}
	if err := writer.transcript.flush(); err != nil {
		return chatIndexState{}, err
	}
	writer.state.indexedBytes = offset
	return writer.state, nil
}

func (writer *chatIndexWriter) indexRecord(raw []byte, startOffset, endOffset int64) error {
	line := bytes.TrimSuffix(raw, []byte{'\n'})
	line = bytes.TrimSuffix(line, []byte{'\r'})
	if len(line) == 0 {
		return nil
	}
	writer.state.eventOrdinal++
	if len(line) > maxEventRecordBytes {
		return fmt.Errorf("event record exceeds %d bytes", maxEventRecordBytes)
	}
	// Records written before sequence numbers fall back to their ordinal.
	event, err := decodeStoredEvent(line, writer.state.eventOrdinal)
	if err != nil {
		return nil
	}
	return writer.foldEvent(event, startOffset, endOffset)
}

// indexEvent folds one event read from chats.sqlite into the projection. The
// sequence column is authoritative, so no byte range accompanies it.
func (writer *chatIndexWriter) indexEvent(event servicechat.Event) error {
	writer.state.eventOrdinal++
	return writer.foldEvent(event, 0, 0)
}

// foldEvent records an already-decoded event in the projection. The caller
// supplies the source location: a byte range for the JSONL log, or zeros when
// the sequence addresses the event.
func (writer *chatIndexWriter) foldEvent(
	event servicechat.Event,
	startOffset int64,
	endOffset int64,
) error {
	if event.Seq > writer.state.lastSeq {
		writer.state.lastSeq = event.Seq
	}
	if writer.eventOffsets != nil {
		if _, err := writer.eventOffsets.ExecContext(
			writer.ctx,
			writer.chatID,
			writer.state.eventOrdinal,
			event.Seq,
			startOffset,
			endOffset-startOffset,
		); err != nil {
			return err
		}
	}
	if err := writer.indexTranscriptTurn(event, startOffset, endOffset); err != nil {
		return err
	}
	return writer.transcript.indexItem(
		event, writer.state.eventOrdinal, writer.turn.ordinal, startOffset, endOffset,
	)
}

func (writer *chatIndexWriter) indexTranscriptTurn(
	event servicechat.Event,
	startOffset int64,
	endOffset int64,
) error {
	startsNew := !writer.turn.hasExisting || servicechat.EventStartsTranscriptTurn(
		writer.turn.sourceID,
		writer.turn.hasUser,
		true,
		event,
	)
	if startsNew {
		writer.turn.ordinal++
		writer.turn.sourceID = event.TurnID
		writer.turn.hasUser = event.Type == "user"
		writer.turn.hasExisting = true
		if writer.sourceBySeq {
			_, err := writer.insertTurn.ExecContext(
				writer.ctx,
				writer.chatID,
				writer.turn.ordinal,
				writer.turn.sourceID,
				writer.turn.hasUser,
				event.Seq,
				event.Seq,
			)
			return err
		}
		_, err := writer.insertTurn.ExecContext(
			writer.ctx,
			writer.chatID,
			writer.turn.ordinal,
			writer.turn.sourceID,
			writer.turn.hasUser,
			event.Seq,
			event.Seq,
			startOffset,
			endOffset,
		)
		return err
	}

	if writer.turn.sourceID == "" && event.TurnID != "" {
		writer.turn.sourceID = event.TurnID
	}
	writer.turn.hasUser = writer.turn.hasUser || event.Type == "user"
	if writer.sourceBySeq {
		_, err := writer.updateTurn.ExecContext(
			writer.ctx,
			writer.turn.sourceID,
			writer.turn.hasUser,
			event.Seq,
			writer.chatID,
			writer.turn.ordinal,
		)
		return err
	}
	_, err := writer.updateTurn.ExecContext(
		writer.ctx,
		writer.turn.sourceID,
		writer.turn.hasUser,
		event.Seq,
		endOffset,
		writer.chatID,
		writer.turn.ordinal,
	)
	return err
}
