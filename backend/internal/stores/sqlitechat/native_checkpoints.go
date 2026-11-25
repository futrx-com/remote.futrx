package sqlitechat

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	chat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
)

const nativeCheckpointSubtype = "native_checkpoint"

// A successful delivery records an immutable native boundary in the event
// history it covers. The cursor is interpreted only by its harness adapter.
// Keeping it in that history makes normal branch cutoffs apply automatically.
type nativeCheckpoint struct {
	Version        int    `json:"version"`
	SessionID      string `json:"sessionId"`
	ConversationID string `json:"conversationId"`
	Cursor         string `json:"cursor"`
	ThroughSeq     int64  `json:"throughSeq"`
}

func (s *Store) saveNativeCheckpoint(ctx context.Context, tx *sql.Tx, run chat.RunReference, session sessionState, tail int64, cursor string) (int64, error) {
	var timestamp int64
	var turnID string
	if err := tx.QueryRowContext(ctx, "SELECT t,turn_id FROM events WHERE branch_id=? AND seq=?", run.BranchID, tail).Scan(&timestamp, &turnID); err != nil {
		return 0, err
	}
	checkpoint := nativeCheckpoint{Version: 1, SessionID: session.NativeSessionID,
		ConversationID: session.NativeConversationID, Cursor: cursor, ThroughSeq: tail}
	raw, err := json.Marshal(checkpoint)
	if err != nil {
		return 0, err
	}
	event, err := s.appendEventTx(ctx, tx, run.BranchID, chat.Event{
		T: timestamp, TurnID: turnID, Type: "system", Subtype: nativeCheckpointSubtype,
		Provider: run.Provider, AccountID: run.AccountID, Data: raw,
	}, nil)
	return event.Seq, err
}

func retainedNativeCheckpoint(ctx context.Context, q sqlQuerier, branch string, through int64, provider chat.Provider, account string) (nativeCheckpoint, error) {
	var raw []byte
	var seq int64
	err := q.QueryRowContext(ctx, lineageSQL+`SELECT e.seq,e.payload_json FROM lineage l JOIN events e ON e.branch_id=l.id
	 WHERE e.seq<=l.max_seq AND e.event_type='system'
	 AND json_extract(e.payload_json,'$.subtype')='native_checkpoint'
	 AND json_extract(e.payload_json,'$.provider')=?
	 AND coalesce(json_extract(e.payload_json,'$.accountId'),'')=?
	 AND json_extract(e.payload_json,'$.data.version')=1
	 ORDER BY e.seq DESC LIMIT 1`, branch, through, provider, account).Scan(&seq, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nativeCheckpoint{}, nil
	}
	if err != nil {
		return nativeCheckpoint{}, err
	}
	var event chat.Event
	if err = json.Unmarshal(raw, &event); err != nil {
		return nativeCheckpoint{}, err
	}
	var checkpoint nativeCheckpoint
	if err = json.Unmarshal(event.Data, &checkpoint); err != nil {
		return nativeCheckpoint{}, err
	}
	if checkpoint.SessionID == "" || checkpoint.Cursor == "" || checkpoint.ConversationID == "" || checkpoint.ThroughSeq < 0 || checkpoint.ThroughSeq >= seq {
		return nativeCheckpoint{}, nil
	}
	// Include the hidden checkpoint itself in the safe prefix. It is never
	// transferred as portable conversation material.
	checkpoint.ThroughSeq = seq
	return checkpoint, nil
}
