package filechat

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	configconstants "github.com/futrx-com/remote.futrx.com/internal/config/constants"
	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
)

// The SQLite backend keeps its transcript projection inside chats.sqlite
// instead of the disposable transcript-index.sqlite database. The projected
// turns and items are identical; what changes is the source they are built
// from. Byte offsets address the JSONL log, while chats.sqlite addresses an
// event by its sequence, so locators are sequences and progress is measured in
// projected payload bytes rather than indexed file bytes.

// errChatStoreUnavailable reports that the configured event database is not
// open, which is the SQLite counterpart of an unavailable transcript index.
var errChatStoreUnavailable = errors.New("chat store is unavailable")

func (s *Store) sqliteDB() (*chatStoreDB, error) {
	if s.sqlite == nil {
		return nil, errChatStoreUnavailable
	}
	return s.sqlite, nil
}

func (s *Store) deleteSQLiteProjection(ctx context.Context, id servicechat.ID) error {
	store, err := s.sqliteDB()
	if err != nil {
		return err
	}
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := deleteChatRows(ctx, tx, id, chatSQLiteProjectionTables[:]); err != nil {
		return err
	}
	return tx.Commit()
}

// readSQLiteProjectionState reports how much of the stored event stream has
// been folded into the projected turns and items. It reuses the JSONL index
// checkpoint shape: indexedBytes is projected payload bytes, and the
// file-specific fields are unused.
func (s *Store) readSQLiteProjectionState(
	ctx context.Context,
	id servicechat.ID,
) (chatIndexState, bool, error) {
	store, err := s.sqliteDB()
	if err != nil {
		return chatIndexState{}, false, err
	}
	state := newChatIndexState()
	err = store.db.QueryRowContext(ctx, `
		SELECT event_ordinal, last_seq, projected_bytes
		FROM chat_transcript_projection_state
		WHERE chat_id = ?`, id,
	).Scan(&state.eventOrdinal, &state.lastSeq, &state.indexedBytes)
	if errors.Is(err, sql.ErrNoRows) {
		return chatIndexState{}, false, nil
	}
	if err != nil {
		return chatIndexState{}, false, err
	}
	return state, true, nil
}

func writeSQLiteProjectionState(
	ctx context.Context,
	tx *sql.Tx,
	id servicechat.ID,
	state chatIndexState,
) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO chat_transcript_projection_state
			(chat_id, event_ordinal, last_seq, projected_bytes)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(chat_id) DO UPDATE SET
			event_ordinal = excluded.event_ordinal,
			last_seq = excluded.last_seq,
			projected_bytes = excluded.projected_bytes`,
		id, state.eventOrdinal, state.lastSeq, state.indexedBytes,
	)
	return err
}

// sqliteEventProgress reports the stored tail sequence and the total payload
// bytes a projection has to cover. Both are O(1): the sequence comes from the
// unique index and the byte total is maintained by write triggers.
func (s *Store) sqliteEventProgress(
	ctx context.Context,
	id servicechat.ID,
) (lastSeq int64, totalBytes int64, err error) {
	store, err := s.sqliteDB()
	if err != nil {
		return 0, 0, err
	}
	if err := store.db.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(seq), 0) FROM chat_events WHERE chat_id = ?`, id,
	).Scan(&lastSeq); err != nil {
		return 0, 0, err
	}
	err = store.db.QueryRowContext(ctx,
		`SELECT payload_bytes FROM chat_event_bytes WHERE chat_id = ?`, id,
	).Scan(&totalBytes)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	return lastSeq, totalBytes, err
}

// syncSQLiteTranscript projects every stored event the read model has not
// seen yet. Callers hold the chat lock, so the checkpoint only races the
// background writer for the same chat, never a concurrent append.
func (s *Store) syncSQLiteTranscript(
	ctx context.Context,
	id servicechat.ID,
) (chatIndexState, error) {
	if _, err := s.sqliteDB(); err != nil {
		return chatIndexState{}, err
	}
	for {
		if err := ctx.Err(); err != nil {
			return chatIndexState{}, err
		}
		state, found, err := s.readSQLiteProjectionState(ctx, id)
		if err != nil {
			return chatIndexState{}, err
		}
		lastSeq, totalBytes, err := s.sqliteEventProgress(ctx, id)
		if err != nil {
			return chatIndexState{}, err
		}
		if found && state.lastSeq == lastSeq && state.indexedBytes == totalBytes {
			return state, nil
		}
		// A checkpoint ahead of the stored stream, or one that agrees on the
		// tail but not on the bytes, describes a rewritten stream. Rebuild
		// instead of appending to a projection of different history.
		if !found ||
			state.lastSeq > lastSeq ||
			state.indexedBytes > totalBytes ||
			(state.lastSeq == lastSeq) != (state.indexedBytes == totalBytes) {
			if err := s.deleteSQLiteProjection(ctx, id); err != nil {
				return chatIndexState{}, err
			}
			state = newChatIndexState()
		}
		consumed, next, err := s.projectSQLiteBatch(ctx, id, state)
		if err != nil {
			return chatIndexState{}, err
		}
		if !consumed {
			// Everything stored is already projected; the remaining mismatch
			// only reports inconsistent bookkeeping, which this pass cannot
			// repair by reading more events.
			return next, nil
		}
	}
}

// projectSQLiteBatch folds at most one checkpoint worth of events into the
// projection inside a single transaction.
func (s *Store) projectSQLiteBatch(
	ctx context.Context,
	id servicechat.ID,
	state chatIndexState,
) (bool, chatIndexState, error) {
	store, err := s.sqliteDB()
	if err != nil {
		return false, chatIndexState{}, err
	}
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return false, chatIndexState{}, err
	}
	defer func() { _ = tx.Rollback() }()

	consumed, next, err := s.writeSQLiteProjectionBatch(ctx, store.db, tx, id, state)
	if err != nil {
		return false, chatIndexState{}, err
	}
	if err := writeSQLiteProjectionState(ctx, tx, id, next); err != nil {
		return false, chatIndexState{}, err
	}
	if err := tx.Commit(); err != nil {
		return false, chatIndexState{}, err
	}
	return consumed, next, nil
}

func (s *Store) writeSQLiteProjectionBatch(
	ctx context.Context,
	db *sql.DB,
	tx *sql.Tx,
	id servicechat.ID,
	state chatIndexState,
) (bool, chatIndexState, error) {
	turn, err := readLastIndexedTurn(ctx, tx, id)
	if err != nil {
		return false, chatIndexState{}, err
	}
	writer := newChatIndexWriter(ctx, tx, id, state, turn)
	writer.sourceBySeq = true
	if err := writer.prepare(); err != nil {
		return false, chatIndexState{}, err
	}
	defer writer.close()

	// The read cursor and the projection write use separate pooled
	// connections, so the batch is bounded only by its byte budget.
	rows, err := db.QueryContext(ctx, `
		SELECT seq, payload FROM chat_events
		WHERE chat_id = ? AND seq > ?
		ORDER BY seq`, id, state.lastSeq)
	if err != nil {
		return false, chatIndexState{}, err
	}

	var batchBytes int64
	consumed := false
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			_ = rows.Close()
			return false, chatIndexState{}, err
		}
		var seq int64
		var payload []byte
		if err := rows.Scan(&seq, &payload); err != nil {
			_ = rows.Close()
			return false, chatIndexState{}, err
		}
		event, err := decodeStoredEvent(payload, seq)
		if err != nil {
			_ = rows.Close()
			return false, chatIndexState{}, fmt.Errorf(
				"%w: decode stored event %d: %v", errInvalidChatEventIndex, seq, err,
			)
		}
		if event.Seq != seq {
			_ = rows.Close()
			return false, chatIndexState{}, fmt.Errorf(
				"%w: stored sequence %d, record has %d", errInvalidChatEventIndex, seq, event.Seq,
			)
		}
		if err := writer.indexEvent(event); err != nil {
			_ = rows.Close()
			return false, chatIndexState{}, err
		}
		consumed = true
		batchBytes += int64(len(payload))
		if batchBytes >= configconstants.ChatIndexCheckpointBytes {
			break
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return false, chatIndexState{}, err
	}
	if err := rows.Close(); err != nil {
		return false, chatIndexState{}, err
	}
	if err := writer.transcript.flush(); err != nil {
		return false, chatIndexState{}, err
	}

	next := writer.state
	next.indexedBytes = state.indexedBytes + batchBytes
	next.fileMtimeNS = 0
	next.prefixHash = indexPrefixHashOffset64
	next.tailComplete = true
	return consumed, next, nil
}

func (s *Store) readSQLiteTranscriptPage(
	ctx context.Context,
	id servicechat.ID,
	query servicechat.TranscriptPageQuery,
) (servicechat.TranscriptPage, error) {
	if _, err := s.sqliteDB(); err != nil {
		return servicechat.TranscriptPage{}, fmt.Errorf("%w: %v",
			servicechat.ErrTranscriptProjectionUnavailable, err)
	}
	lastSeq, totalBytes, err := s.sqliteEventProgress(ctx, id)
	if err != nil {
		return servicechat.TranscriptPage{}, err
	}
	state, found, err := s.readSQLiteProjectionState(ctx, id)
	if err != nil {
		return servicechat.TranscriptPage{}, err
	}
	// Serving a page whose archive has not been folded in yet would present a
	// partial conversation as complete, so readiness covers the import too.
	imported, archiveBytes, err := s.archiveImported(ctx, id)
	if err != nil {
		return servicechat.TranscriptPage{}, err
	}
	if !found || state.lastSeq != lastSeq || state.indexedBytes != totalBytes || !imported {
		s.startTranscriptIndex(id)
		indexedBytes := state.indexedBytes
		if indexedBytes < 0 || indexedBytes > totalBytes {
			indexedBytes = 0
		}
		// While the archive is still streaming in, the tail sequence has to
		// come from the file itself, exactly as the JSONL projection does, so
		// the live stream can attach before the backfill finishes.
		tailSeqKnown := imported
		if !imported && archiveBytes > 0 {
			if tailSeq, tailErr := lastStoredEventSeq(s.eventsPath(id), archiveBytes); tailErr == nil && tailSeq > 0 {
				if tailSeq > lastSeq {
					lastSeq = tailSeq
				}
				tailSeqKnown = true
			}
		}
		return servicechat.TranscriptPage{
			Turns:   []servicechat.TranscriptTurn{},
			LastSeq: lastSeq,
			Indexing: &servicechat.TranscriptIndexProgress{
				IndexedBytes: indexedBytes,
				TotalBytes:   totalBytes,
				TailSeqKnown: tailSeqKnown,
			},
		}, nil
	}
	store, err := s.sqliteDB()
	if err != nil {
		return servicechat.TranscriptPage{}, fmt.Errorf("%w: %v",
			servicechat.ErrTranscriptProjectionUnavailable, err)
	}
	return readProjectedTranscriptPage(ctx, store.db, id, state, query)
}

func (s *Store) readSQLiteTranscriptContent(
	ctx context.Context,
	id servicechat.ID,
	contentID string,
	afterBytes int64,
	limitBytes int,
) (servicechat.TranscriptContentPage, error) {
	store, err := s.sqliteDB()
	if err != nil {
		return servicechat.TranscriptContentPage{}, fmt.Errorf("%w: %v",
			servicechat.ErrTranscriptProjectionUnavailable, err)
	}
	limitBytes = transcriptContentLimit(limitBytes)

	ref, err := readSQLiteContentRef(ctx, store.db, id, contentID)
	if err != nil {
		return servicechat.TranscriptContentPage{}, err
	}
	var payload []byte
	err = store.db.QueryRowContext(ctx, `
		SELECT payload FROM chat_events
		WHERE chat_id = ? AND seq = ?`, id, ref.sourceSeq,
	).Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		return servicechat.TranscriptContentPage{}, fmt.Errorf(
			"%w: content source event %d", errInvalidChatEventIndex, ref.sourceSeq,
		)
	}
	if err != nil {
		return servicechat.TranscriptContentPage{}, err
	}
	event, err := decodeStoredEvent(payload, ref.sourceSeq)
	if err != nil {
		return servicechat.TranscriptContentPage{}, fmt.Errorf(
			"%w: decode content source: %v", errInvalidChatEventIndex, err,
		)
	}
	return transcriptContentResult(contentID, ref, event, afterBytes, limitBytes)
}

func readSQLiteContentRef(
	ctx context.Context,
	db *sql.DB,
	id servicechat.ID,
	contentID string,
) (transcriptContentRef, error) {
	ref := transcriptContentRef{id: contentID}
	err := db.QueryRowContext(ctx, `
		SELECT field_kind, field_key, source_seq, content_bytes
		FROM chat_transcript_content_refs
		WHERE chat_id = ? AND content_id = ?`, id, contentID,
	).Scan(&ref.fieldKind, &ref.fieldKey, &ref.sourceSeq, &ref.contentBytes)
	if errors.Is(err, sql.ErrNoRows) {
		return transcriptContentRef{}, servicechat.ErrTranscriptContentNotFound
	}
	return ref, err
}

// readSQLiteTranscriptWindow selects the newest turns before beforeSeq and
// reads their whole sequence range, mirroring the offset-indexed window with
// sequences instead of byte ranges.
func (s *Store) readSQLiteTranscriptWindow(
	ctx context.Context,
	id servicechat.ID,
	beforeSeq int64,
	turnLimit int,
) (servicechat.TranscriptEventWindow, error) {
	store, err := s.sqliteDB()
	if err != nil {
		return servicechat.TranscriptEventWindow{}, err
	}
	// The caller holds the chat lock, which is where the archive import runs.
	if err := s.events.Prepare(ctx, id); err != nil {
		return servicechat.TranscriptEventWindow{}, err
	}
	state, err := s.syncSQLiteTranscript(ctx, id)
	if err != nil {
		return servicechat.TranscriptEventWindow{}, err
	}

	if turnLimit <= 0 {
		turnLimit = 1
	}
	rows, err := store.db.QueryContext(ctx, `
		SELECT start_seq, end_seq
		FROM chat_transcript_turns
		WHERE chat_id = ? AND (? <= 0 OR start_seq < ?)
		ORDER BY turn_ordinal DESC
		LIMIT ?`, id, beforeSeq, beforeSeq, turnLimit+1,
	)
	if err != nil {
		return servicechat.TranscriptEventWindow{}, err
	}
	var firstSeq, lastSeq int64 = -1, -1
	for rows.Next() {
		var startSeq, endSeq int64
		if err := rows.Scan(&startSeq, &endSeq); err != nil {
			_ = rows.Close()
			return servicechat.TranscriptEventWindow{}, err
		}
		if endSeq < startSeq {
			_ = rows.Close()
			return servicechat.TranscriptEventWindow{}, fmt.Errorf(
				"%w: transcript turn range", errInvalidChatEventIndex,
			)
		}
		if firstSeq < 0 || startSeq < firstSeq {
			firstSeq = startSeq
		}
		if endSeq > lastSeq {
			lastSeq = endSeq
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return servicechat.TranscriptEventWindow{}, err
	}
	if err := rows.Close(); err != nil {
		return servicechat.TranscriptEventWindow{}, err
	}

	window := servicechat.TranscriptEventWindow{LastSeq: state.lastSeq}
	if firstSeq < 0 {
		return window, nil
	}
	eventRows, err := store.db.QueryContext(ctx, `
		SELECT seq, payload FROM chat_events
		WHERE chat_id = ? AND seq >= ? AND seq <= ?
		ORDER BY seq`, id, firstSeq, lastSeq,
	)
	if err != nil {
		return servicechat.TranscriptEventWindow{}, err
	}
	defer func() { _ = eventRows.Close() }()
	for eventRows.Next() {
		event, err := scanEventRow(eventRows)
		if err != nil {
			return servicechat.TranscriptEventWindow{}, err
		}
		window.Events = append(window.Events, event)
	}
	return window, eventRows.Err()
}
