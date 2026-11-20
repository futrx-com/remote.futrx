package sqlitechat

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/futrx-com/remote.futrx.com/internal/model/conversation"
	chat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
)

// One entry per provider/account describes only the current branch's resumable
// native session. Attempts and delivered prompts are not separate stored history.
type sessionState struct {
	NativeSessionID      string `json:"nativeSessionId,omitempty"`
	NativeConversationID string `json:"nativeConversationId,omitempty"`
	ForkFromSessionID    string `json:"forkFromSessionId,omitempty"`
	HistoryThroughSeq    int64  `json:"historyThroughSeq"`
	State                string `json:"state"`
	RunID                string `json:"runId,omitempty"`
	DeliveryMethod       string `json:"deliveryMethod,omitempty"`
}

func sessionKey(provider chat.Provider, account string) string {
	key, _ := json.Marshal([2]string{string(provider), account})
	return string(key)
}

var _ chat.ConversationRuntime = (*Store)(nil)

// SeedLegacySessionsTx makes only the migration assumption selected by the
// operator: the last assigned harness owns the imported prefix.
func SeedLegacySessionsTx(ctx context.Context, tx *sql.Tx, meta chat.Meta, lastSeq int64) error {
	nativeID := meta.SessionID(meta.Provider)
	if nativeID == "" {
		return nil
	}
	return writeSessionsTx(ctx, tx, meta.ID, map[string]sessionState{
		sessionKey(meta.Provider, meta.AccountID): {
			NativeSessionID: nativeID, HistoryThroughSeq: lastSeq, State: "legacy_assumed",
		},
	})
}

func readSessions(ctx context.Context, q sqlQuerier, id chat.ID) (string, int64, map[string]sessionState, error) {
	var branch string
	var tail int64
	var raw []byte
	err := q.QueryRowContext(ctx, `SELECT c.active_branch_id,b.last_seq,c.sessions_json
        FROM chats c JOIN branches b ON b.id=c.active_branch_id WHERE c.id=?`, id).Scan(&branch, &tail, &raw)
	if err != nil {
		return "", 0, nil, err
	}
	sessions := map[string]sessionState{}
	if err = json.Unmarshal(raw, &sessions); err != nil {
		return "", 0, nil, fmt.Errorf("decode conversation sessions: %w", err)
	}
	if sessions == nil {
		sessions = map[string]sessionState{}
	}
	return branch, tail, sessions, nil
}

func writeSessionsTx(ctx context.Context, tx *sql.Tx, id chat.ID, sessions map[string]sessionState) error {
	raw, err := json.Marshal(sessions)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, "UPDATE chats SET sessions_json=? WHERE id=?", raw, id)
	return err
}

func (s *Store) PrepareRun(ctx context.Context, input chat.RunPreparation) (chat.PreparedRun, error) {
	// Reserve the run before loading its immutable history range, keeping the
	// writer lock out of potentially large history reads.
	s.mu.Lock()
	prepared, err := s.prepareRunLocked(ctx, input)
	s.mu.Unlock()
	if err != nil {
		return chat.PreparedRun{}, err
	}
	events, err := readPortableHistoryRange(ctx, s.db, prepared.History.BranchID, prepared.History.AfterSeq, prepared.History.ThroughSeq)
	if err != nil {
		finishErr := s.FinishRun(context.WithoutCancel(ctx), prepared.RunReference, chat.RunOutcome{Status: "failed"})
		return chat.PreparedRun{}, errors.Join(err, finishErr)
	}
	prepared.History.Records = chat.ConversationRecords(events)
	return prepared, nil
}

func (s *Store) prepareRunLocked(ctx context.Context, input chat.RunPreparation) (chat.PreparedRun, error) {
	if input.RunID == "" {
		return chat.PreparedRun{}, errors.New("run ID is required")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return chat.PreparedRun{}, err
	}
	defer tx.Rollback()
	branch, tail, sessions, err := readSessions(ctx, tx, input.ChatID)
	if err != nil {
		return chat.PreparedRun{}, err
	}
	if input.ThroughSeq != nil {
		if *input.ThroughSeq < 0 || *input.ThroughSeq > tail {
			return chat.PreparedRun{}, errors.New("history boundary is outside the active branch")
		}
		tail = *input.ThroughSeq
	}
	key := sessionKey(input.Provider, input.AccountID)
	session, exists := sessions[key]
	var checkpoint string
	if !exists && input.AllowNativeFork && !input.ForceFresh {
		// Only a new branch can inherit a native session. An uncertain run on an
		// existing branch must still use the normal fresh-session recovery path.
		var parent sql.NullString
		if err = tx.QueryRowContext(ctx, "SELECT parent_branch_id FROM branches WHERE id=?", branch).Scan(&parent); err != nil {
			return chat.PreparedRun{}, err
		}
		if parent.Valid {
			var saved nativeCheckpoint
			saved, err = retainedNativeCheckpoint(ctx, tx, branch, tail, input.Provider, input.AccountID)
			if err != nil {
				return chat.PreparedRun{}, err
			}
			if saved.Cursor != "" {
				checkpoint = saved.Cursor
				session = sessionState{NativeSessionID: saved.SessionID, NativeConversationID: saved.ConversationID,
					HistoryThroughSeq: saved.ThroughSeq, State: "ready", ForkFromSessionID: saved.SessionID}
			}
		}
	}
	// Pending/accepted input may already have reached the native harness. After
	// an interrupted run, start fresh instead of replaying into that session.
	if input.ForceFresh || session.RunID != "" ||
		(session.State != "ready" && session.State != "legacy_assumed") || session.NativeSessionID == "" {
		session = sessionState{}
	}
	if session.NativeConversationID == "" {
		session.NativeConversationID = string(input.ChatID)
	}
	if session.HistoryThroughSeq > tail {
		return chat.PreparedRun{}, fmt.Errorf("session bookmark %d is after history boundary %d", session.HistoryThroughSeq, tail)
	}
	prepared := chat.PreparedRun{
		RunReference:         chat.RunReference{ChatID: input.ChatID, BranchID: branch, RunID: input.RunID, Provider: input.Provider, AccountID: input.AccountID},
		ResumeID:             session.NativeSessionID,
		NativeCheckpoint:     checkpoint,
		NativeConversationID: session.NativeConversationID,
		History:              conversation.History{BranchID: branch, AfterSeq: session.HistoryThroughSeq, ThroughSeq: tail},
	}
	session.State, session.RunID, session.DeliveryMethod = "pending", input.RunID, ""
	sessions[key] = session
	if err = writeSessionsTx(ctx, tx, input.ChatID, sessions); err != nil {
		return chat.PreparedRun{}, err
	}
	return prepared, tx.Commit()
}

// updateRunSession ignores callbacks from a replaced run or branch. The token
// comparison and update share a transaction, so late completion cannot make a
// different native session resumable.
func (s *Store) updateRunSession(ctx context.Context, run chat.RunReference, update func(*sql.Tx, *sessionState, int64) error) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	branch, tail, sessions, err := readSessions(ctx, tx, run.ChatID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	key := sessionKey(run.Provider, run.AccountID)
	session := sessions[key]
	if branch != run.BranchID || run.RunID == "" || session.RunID != run.RunID {
		return false, nil
	}
	if err = update(tx, &session, tail); err != nil {
		return false, err
	}
	sessions[key] = session
	if err = writeSessionsTx(ctx, tx, run.ChatID, sessions); err != nil {
		return false, err
	}
	return true, tx.Commit()
}

func (s *Store) RecordRunInput(ctx context.Context, run chat.RunReference, method string) error {
	if method == "" {
		return errors.New("run input delivery method is required")
	}
	current, err := s.updateRunSession(ctx, run, func(_ *sql.Tx, session *sessionState, _ int64) error {
		if session.State != "pending" {
			return errors.New("run is not pending input delivery")
		}
		session.DeliveryMethod = method
		return nil
	})
	if err == nil && !current {
		return errors.New("run is not pending input delivery")
	}
	return err
}

func (s *Store) AcceptRunInput(ctx context.Context, run chat.RunReference, evidence chat.InputAcceptance) error {
	if evidence.Evidence == "" {
		return errors.New("input acceptance requires provider evidence")
	}
	_, err := s.updateRunSession(ctx, run, func(_ *sql.Tx, session *sessionState, _ int64) error {
		if session.State == "accepted" {
			return nil
		}
		if session.State != "pending" || session.DeliveryMethod == "" {
			return errors.New("run input was not recorded before acceptance")
		}
		if session.ForkFromSessionID != "" && (evidence.NativeSessionID == "" || evidence.NativeSessionID == session.ForkFromSessionID) {
			return errors.New("native fork did not identify a new session")
		}
		session.State = "accepted"
		if evidence.NativeSessionID != "" {
			session.NativeSessionID = evidence.NativeSessionID
		}
		// The safe bookmark moves only when this accepted run completes.
		return nil
	})
	return err
}

func (s *Store) FinishRun(ctx context.Context, run chat.RunReference, outcome chat.RunOutcome) error {
	_, err := s.updateRunSession(ctx, run, func(tx *sql.Tx, session *sessionState, tail int64) error {
		if outcome.NativeSessionID != "" {
			session.NativeSessionID = outcome.NativeSessionID
		}
		if outcome.Status == "completed" && session.State == "accepted" && session.NativeSessionID != "" && session.NativeSessionID != session.ForkFromSessionID {
			if outcome.NativeCheckpoint != "" {
				var err error
				tail, err = s.saveNativeCheckpoint(ctx, tx, run, *session, tail, outcome.NativeCheckpoint)
				if err != nil {
					return err
				}
			}
			session.State, session.HistoryThroughSeq = "ready", tail
		} else {
			session.State = "uncertain"
		}
		session.RunID, session.DeliveryMethod = "", ""
		session.ForkFromSessionID = ""
		return nil
	})
	return err
}

// Select portable records before decoding. Provider telemetry/native-only rows
// can dwarf the visible conversation and must not be read into the handoff.
func readPortableHistoryRange(ctx context.Context, q sqlQuerier, branch string, after, through int64) ([]chat.Event, error) {
	rows, err := q.QueryContext(ctx, lineageSQL+`SELECT e.seq,json_remove(e.payload_json,'$.raw','$.native')
 FROM lineage l JOIN events e ON e.branch_id=l.id
 WHERE e.seq<=l.max_seq AND e.seq>?
 AND e.event_type IN ('user','assistant_text','tool_use_start','tool_use_end','attachment')
 ORDER BY e.seq`, branch, through, after)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return decodeEventRows(rows)
}
