package filechat

import (
	"context"
	"database/sql"
	"errors"

	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
)

type chatIndexState struct {
	indexedBytes int64
	eventOrdinal int64
	lastSeq      int64
	fileMtimeNS  int64
	prefixHash   uint64
	tailComplete bool
}

func newChatIndexState() chatIndexState {
	return chatIndexState{
		prefixHash:   indexPrefixHashOffset64,
		tailComplete: true,
	}
}

type indexedTurnState struct {
	ordinal     int64
	sourceID    string
	hasUser     bool
	hasExisting bool
}

func (index *chatEventIndex) readState(
	ctx context.Context,
	id servicechat.ID,
) (chatIndexState, bool, error) {
	if err := index.availabilityError(); err != nil {
		return chatIndexState{}, false, err
	}
	var state chatIndexState
	var prefixHash int64
	var tailComplete int
	err := index.db.QueryRowContext(ctx, `
		SELECT indexed_bytes, event_ordinal, last_seq, file_mtime_ns,
		       prefix_hash, tail_complete
		FROM chat_event_index_state
		WHERE chat_id = ?`, id,
	).Scan(
		&state.indexedBytes,
		&state.eventOrdinal,
		&state.lastSeq,
		&state.fileMtimeNS,
		&prefixHash,
		&tailComplete,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return chatIndexState{}, false, nil
	}
	if err != nil {
		return chatIndexState{}, false, err
	}
	state.prefixHash = uint64(prefixHash)
	state.tailComplete = tailComplete != 0
	return state, true, nil
}

func writeChatIndexState(
	ctx context.Context,
	tx *sql.Tx,
	id servicechat.ID,
	state chatIndexState,
) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO chat_event_index_state
			(chat_id, indexed_bytes, event_ordinal, last_seq, file_mtime_ns,
			 prefix_hash, tail_complete)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(chat_id) DO UPDATE SET
			indexed_bytes = excluded.indexed_bytes,
			event_ordinal = excluded.event_ordinal,
			last_seq = excluded.last_seq,
			file_mtime_ns = excluded.file_mtime_ns,
			prefix_hash = excluded.prefix_hash,
			tail_complete = excluded.tail_complete`,
		id,
		state.indexedBytes,
		state.eventOrdinal,
		state.lastSeq,
		state.fileMtimeNS,
		int64(state.prefixHash),
		state.tailComplete,
	)
	return err
}

func deleteChatIndexRows(ctx context.Context, tx *sql.Tx, id servicechat.ID) error {
	for _, table := range []string{
		"chat_transcript_content_refs",
		"chat_transcript_items",
		"chat_event_offsets",
		"chat_transcript_turns",
		"chat_event_index_state",
	} {
		if _, err := tx.ExecContext(ctx, "DELETE FROM "+table+" WHERE chat_id = ?", id); err != nil {
			return err
		}
	}
	return nil
}

func readLastIndexedTurn(
	ctx context.Context,
	tx *sql.Tx,
	id servicechat.ID,
) (indexedTurnState, error) {
	var turn indexedTurnState
	err := tx.QueryRowContext(ctx, `
		SELECT turn_ordinal, source_turn_id, has_user
		FROM chat_transcript_turns
		WHERE chat_id = ?
		ORDER BY turn_ordinal DESC
		LIMIT 1`, id,
	).Scan(&turn.ordinal, &turn.sourceID, &turn.hasUser)
	if errors.Is(err, sql.ErrNoRows) {
		return indexedTurnState{}, nil
	}
	if err != nil {
		return indexedTurnState{}, err
	}
	turn.hasExisting = true
	return turn, nil
}
