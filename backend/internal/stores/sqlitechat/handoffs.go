package sqlitechat

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/model/conversation"
	chat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
)

const sessionSchema = `
CREATE TABLE IF NOT EXISTS harness_sessions (
 id TEXT PRIMARY KEY, branch_id TEXT NOT NULL REFERENCES branches(id) ON DELETE CASCADE,
 provider TEXT NOT NULL, account_id TEXT NOT NULL, native_session_id TEXT NOT NULL DEFAULT '',
 history_through_seq INTEGER NOT NULL DEFAULT 0, state TEXT NOT NULL,
 created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS harness_session_current ON harness_sessions(branch_id,provider,account_id)
 WHERE state IN ('ready','legacy_assumed','pending');
CREATE TABLE IF NOT EXISTS runs (
 id TEXT PRIMARY KEY, branch_id TEXT NOT NULL REFERENCES branches(id) ON DELETE CASCADE,
 turn_id TEXT NOT NULL, harness_session_id TEXT NOT NULL REFERENCES harness_sessions(id),
 provider TEXT NOT NULL, account_id TEXT NOT NULL, model TEXT NOT NULL,
 status TEXT NOT NULL, history_through_seq INTEGER NOT NULL,
 input_blob_id TEXT REFERENCES content_blobs(id), delivery_method TEXT NOT NULL DEFAULT '',
 acceptance_evidence TEXT NOT NULL DEFAULT '', native_turn_id TEXT NOT NULL DEFAULT '',
 error TEXT NOT NULL DEFAULT '', created_at INTEGER NOT NULL, completed_at INTEGER
);
CREATE INDEX IF NOT EXISTS runs_session_status ON runs(harness_session_id,status);
CREATE TABLE IF NOT EXISTS handoffs (
 run_id TEXT PRIMARY KEY REFERENCES runs(id) ON DELETE CASCADE,
 harness_session_id TEXT NOT NULL REFERENCES harness_sessions(id),
 from_seq INTEGER NOT NULL, through_seq INTEGER NOT NULL,
 status TEXT NOT NULL, delivery_method TEXT NOT NULL DEFAULT '',
 created_at INTEGER NOT NULL, accepted_at INTEGER
);
`

var _ chat.ConversationRuntime = (*Store)(nil)

// SeedLegacySessionsTx deliberately makes exactly the migration assumption the
// operator selected: the last assigned provider owns the imported prefix. It
// does not claim older mixed-provider history was actually ingested natively.
func SeedLegacySessionsTx(ctx context.Context, tx *sql.Tx, meta chat.Meta, branch string, lastSeq int64) error {
	nativeID := meta.SessionID(meta.Provider)
	if nativeID == "" {
		return nil
	}
	now := time.Now().UnixMilli()
	_, err := tx.ExecContext(ctx, `INSERT INTO harness_sessions
	 (id,branch_id,provider,account_id,native_session_id,history_through_seq,state,created_at,updated_at)
	 VALUES(?,?,?,?,?,?,'legacy_assumed',?,?)`, sessionRecordID(), branch, meta.Provider, meta.AccountID, nativeID, lastSeq, now, now)
	return err
}

func sessionRecordID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		panic(err) // Losing unique session identity is not safely recoverable.
	}
	return hex.EncodeToString(value[:])
}

func (s *Store) PrepareRun(ctx context.Context, input chat.RunPreparation) (chat.PreparedRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return chat.PreparedRun{}, err
	}
	defer tx.Rollback()
	branch, tail, err := activeBranch(ctx, tx, input.ChatID)
	if err != nil {
		return chat.PreparedRun{}, err
	}
	if input.ThroughSeq != nil {
		if *input.ThroughSeq < 0 || *input.ThroughSeq > tail {
			return chat.PreparedRun{}, errors.New("history boundary is outside the active branch")
		}
		tail = *input.ThroughSeq
	}
	now := time.Now().UnixMilli()
	// A previous process may have died after delivery but before recording its
	// outcome. Never replay uncertain input into that same native session.
	_, err = tx.ExecContext(ctx, `UPDATE harness_sessions SET state='uncertain', updated_at=?
	 WHERE branch_id=? AND provider=? AND account_id=? AND id IN
	 (SELECT harness_session_id FROM runs WHERE status IN ('pending','accepted'))`, now, branch, input.Provider, input.AccountID)
	if err != nil {
		return chat.PreparedRun{}, err
	}
	_, err = tx.ExecContext(ctx, `UPDATE runs SET status='uncertain',completed_at=?
	 WHERE status IN ('pending','accepted') AND harness_session_id IN
	 (SELECT id FROM harness_sessions WHERE branch_id=? AND provider=? AND account_id=? AND state='uncertain')`, now, branch, input.Provider, input.AccountID)
	if err != nil {
		return chat.PreparedRun{}, err
	}
	_, err = tx.ExecContext(ctx, `UPDATE handoffs SET status='uncertain' WHERE status='pending' AND run_id IN (SELECT id FROM runs WHERE status='uncertain')`)
	if err != nil {
		return chat.PreparedRun{}, err
	}
	if input.ForceFresh {
		_, err = tx.ExecContext(ctx, `UPDATE harness_sessions SET state='retired',updated_at=?
		 WHERE branch_id=? AND provider=? AND account_id=? AND state IN ('ready','legacy_assumed','pending')`, now, branch, input.Provider, input.AccountID)
		if err != nil {
			return chat.PreparedRun{}, err
		}
	}
	prepared := chat.PreparedRun{RunID: input.RunID}
	var covered int64
	err = tx.QueryRowContext(ctx, `SELECT id,native_session_id,history_through_seq FROM harness_sessions
	 WHERE branch_id=? AND provider=? AND account_id=? AND state IN ('ready','legacy_assumed')`, branch, input.Provider, input.AccountID).
		Scan(&prepared.SessionID, &prepared.ResumeID, &covered)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return chat.PreparedRun{}, err
	}
	if errors.Is(err, sql.ErrNoRows) {
		prepared.SessionID = sessionRecordID()
		_, err = tx.ExecContext(ctx, `INSERT INTO harness_sessions
		 (id,branch_id,provider,account_id,state,created_at,updated_at) VALUES(?,?,?,?,'pending',?,?)`, prepared.SessionID, branch, input.Provider, input.AccountID, now, now)
		if err != nil {
			return chat.PreparedRun{}, err
		}
	}
	if covered > tail {
		return chat.PreparedRun{}, fmt.Errorf("session bookmark %d is after history boundary %d", covered, tail)
	}
	events, err := readPortableHistoryRange(ctx, tx, branch, covered, tail)
	if err != nil {
		return chat.PreparedRun{}, err
	}
	prepared.History = conversation.History{BranchID: branch, AfterSeq: covered, ThroughSeq: tail, Records: chat.ConversationRecords(events)}
	_, err = tx.ExecContext(ctx, `INSERT INTO runs
	 (id,branch_id,turn_id,harness_session_id,provider,account_id,model,status,history_through_seq,created_at)
	 VALUES(?,?,?,?,?,?,?,'pending',?,?)`, input.RunID, branch, input.TurnID, prepared.SessionID, input.Provider, input.AccountID, input.Model, tail, now)
	if err != nil {
		return chat.PreparedRun{}, err
	}
	if tail > covered {
		_, err = tx.ExecContext(ctx, `INSERT INTO handoffs
		 (run_id,harness_session_id,from_seq,through_seq,status,created_at) VALUES(?,?,?,?,'pending',?)`, input.RunID, prepared.SessionID, covered+1, tail, now)
		if err != nil {
			return chat.PreparedRun{}, err
		}
	}
	return prepared, tx.Commit()
}

func (s *Store) RecordRunInput(ctx context.Context, runID, prompt, method string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	blobID, err := putBlobTx(ctx, tx, []byte(prompt), "text/plain; charset=utf-8")
	if err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `UPDATE runs SET input_blob_id=?,delivery_method=? WHERE id=? AND status='pending'`, blobID, method, runID)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return errors.New("run is not pending input delivery")
	}
	if _, err = tx.ExecContext(ctx, `UPDATE handoffs SET delivery_method=? WHERE run_id=?`, method, runID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) AcceptRunInput(ctx context.Context, runID string, evidence chat.InputAcceptance) error {
	if evidence.Evidence == "" {
		return errors.New("input acceptance requires provider evidence")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var sessionID, status, method string
	var through int64
	if err = tx.QueryRowContext(ctx, `SELECT harness_session_id,history_through_seq,status,delivery_method FROM runs WHERE id=?`, runID).Scan(&sessionID, &through, &status, &method); err != nil {
		return err
	}
	if status == "accepted" {
		return nil
	}
	if status != "pending" || method == "" {
		return errors.New("run input was not recorded before acceptance")
	}
	now := time.Now().UnixMilli()
	if _, err = tx.ExecContext(ctx, `UPDATE runs SET status='accepted',acceptance_evidence=?,native_turn_id=? WHERE id=?`, evidence.Evidence, evidence.NativeTurnID, runID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE handoffs SET status='accepted',accepted_at=? WHERE run_id=?`, now, runID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE harness_sessions SET history_through_seq=?,
	 native_session_id=CASE WHEN ?<>'' THEN ? ELSE native_session_id END,updated_at=? WHERE id=?`, through, evidence.NativeSessionID, evidence.NativeSessionID, now, sessionID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) FinishRun(ctx context.Context, runID string, outcome chat.RunOutcome) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var sessionID, branch, status, nativeID string
	if err = tx.QueryRowContext(ctx, `SELECT r.harness_session_id,r.branch_id,r.status,s.native_session_id
	 FROM runs r JOIN harness_sessions s ON s.id=r.harness_session_id WHERE r.id=?`, runID).Scan(&sessionID, &branch, &status, &nativeID); err != nil {
		return err
	}
	if status != "pending" && status != "accepted" {
		// Finalization is idempotent. A late callback from an older process
		// must not alter a session already reconciled by a later preparation.
		return nil
	}
	if outcome.NativeSessionID != "" {
		nativeID = outcome.NativeSessionID
	}
	state := "uncertain"
	if outcome.Status == "completed" && status == "accepted" && nativeID != "" {
		state = "ready"
	}
	if outcome.Status == "" {
		outcome.Status = "uncertain"
	}
	now := time.Now().UnixMilli()
	if _, err = tx.ExecContext(ctx, `UPDATE runs SET status=?,error=?,completed_at=? WHERE id=?`, outcome.Status, outcome.Error, now, runID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE handoffs SET status='uncertain' WHERE run_id=? AND status='pending'`, runID); err != nil {
		return err
	}
	// Only the successful native turn with accepted input owns its produced
	// events. Failed/interrupted runs leave their evidence but cannot be resumed
	// safely without a native reconciliation API; the next run starts fresh.
	if state == "ready" {
		_, err = tx.ExecContext(ctx, `UPDATE harness_sessions SET state=?,native_session_id=?,
		 history_through_seq=(SELECT last_seq FROM branches WHERE id=?),updated_at=? WHERE id=?`, state, nativeID, branch, now, sessionID)
	} else {
		_, err = tx.ExecContext(ctx, `UPDATE harness_sessions SET state=?,native_session_id=?,updated_at=? WHERE id=?`, state, nativeID, now, sessionID)
	}
	if err != nil {
		return err
	}
	return tx.Commit()
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
