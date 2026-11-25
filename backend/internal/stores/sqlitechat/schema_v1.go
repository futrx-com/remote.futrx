package sqlitechat

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	chat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
)

func upgradeSchemaV1(ctx context.Context, tx *sql.Tx) error {
	before, err := canonicalSchemaDigest(ctx, tx)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `
ALTER TABLE chats ADD COLUMN sessions_json BLOB NOT NULL DEFAULT '{}';
ALTER TABLE branches ADD COLUMN last_turn_id TEXT NOT NULL DEFAULT '';
ALTER TABLE branches ADD COLUMN last_turn_start_seq INTEGER NOT NULL DEFAULT 0;
ALTER TABLE branches ADD COLUMN last_turn_has_user INTEGER NOT NULL DEFAULT 0;
ALTER TABLE branches ADD COLUMN search_seq INTEGER NOT NULL DEFAULT 0;
ALTER TABLE branches ADD COLUMN search_pending_json BLOB NOT NULL DEFAULT '{}';
ALTER TABLE events ADD COLUMN turn_ordinal INTEGER NOT NULL DEFAULT 0;
ALTER TABLE events ADD COLUMN turn_start_seq INTEGER NOT NULL DEFAULT 0;
ALTER TABLE events ADD COLUMN turn_key TEXT NOT NULL DEFAULT '';
ALTER TABLE events ADD COLUMN item_key TEXT NOT NULL DEFAULT '';
ALTER TABLE events ADD COLUMN item_mode INTEGER NOT NULL DEFAULT 0;
ALTER TABLE events ADD COLUMN item_start_seq INTEGER NOT NULL DEFAULT 0;
ALTER TABLE events ADD COLUMN visible INTEGER NOT NULL DEFAULT 0;
UPDATE branches SET
 last_turn_id=coalesce((SELECT source_turn_id FROM turns WHERE branch_id=branches.id ORDER BY ordinal DESC LIMIT 1),''),
 last_turn_start_seq=coalesce((SELECT start_seq FROM turns WHERE branch_id=branches.id ORDER BY ordinal DESC LIMIT 1),0),
 last_turn_has_user=coalesce((SELECT has_user FROM turns WHERE branch_id=branches.id ORDER BY ordinal DESC LIMIT 1),0);
`); err != nil {
		return err
	}
	if err = migrateEventFields(ctx, tx); err != nil {
		return err
	}
	if err = migrateSessionMaps(ctx, tx); err != nil {
		return err
	}
	after, err := canonicalSchemaDigest(ctx, tx)
	if err != nil {
		return err
	}
	if before != after {
		return errors.New("canonical conversation data changed during schema migration")
	}
	// These contain derived snapshots or obsolete delivery bookkeeping. All
	// canonical data and imported-source checkpoints remain in their own tables.
	if _, err = tx.ExecContext(ctx, `
DROP TABLE handoffs;
DROP TABLE runs;
DROP TABLE harness_sessions;
DROP TABLE transcript_content_refs;
DROP TABLE content_blobs;
DROP TABLE transcript_items;
DROP TABLE transcript_text_pending;
DROP TABLE transcript_text_runs;
DROP TABLE transcript_text_search;
DROP TABLE transcript_search;
DROP TABLE turns;
DROP INDEX events_by_time;
`+eventIndexSchema+searchSchema); err != nil {
		return err
	}
	if err = verifySchemaDatabase(ctx, tx); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version=%d", schemaVersion))
	return err
}

const schemaBackfillBatchBytes = 4 * 1024 * 1024

func migrateEventFields(ctx context.Context, tx *sql.Tx) error {
	var overlaps int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM (
 SELECT start_seq,lag(end_seq) OVER (PARTITION BY branch_id ORDER BY start_seq) previous_end FROM turns
 ) WHERE previous_end>=start_seq`).Scan(&overlaps); err != nil {
		return err
	}
	if overlaps != 0 {
		return errors.New("overlapping legacy turn ranges")
	}
	var afterBranch string
	var afterSeq int64
	// A bounded page prevents a multi-gigabyte history from becoming a single
	// allocation. The existing turn ranges are authoritative grouping metadata.
	type record struct {
		branch, turn        string
		seq, ordinal, start int64
		raw                 []byte
	}
	var currentBranch string
	var currentOrdinal int64
	itemStarts := map[string]int64{}
	for {
		rows, err := tx.QueryContext(ctx, `SELECT e.branch_id,e.seq,e.payload_json,t.ordinal,t.start_seq,t.source_turn_id
 FROM events e LEFT JOIN turns t ON t.branch_id=e.branch_id AND t.ordinal=(
 SELECT ordinal FROM turns WHERE branch_id=e.branch_id AND start_seq<=e.seq ORDER BY start_seq DESC LIMIT 1
 ) AND e.seq<=t.end_seq
 WHERE (e.branch_id,e.seq)>(?,?)
 ORDER BY e.branch_id,e.seq LIMIT 128`, afterBranch, afterSeq)
		if err != nil {
			return err
		}
		var page []record
		pageBytes := 0
		for rows.Next() {
			var r record
			if err = rows.Scan(&r.branch, &r.seq, &r.raw, &r.ordinal, &r.start, &r.turn); err != nil {
				rows.Close()
				return fmt.Errorf("event has missing or invalid turn grouping: %w", err)
			}
			page = append(page, r)
			pageBytes += len(r.raw)
			// Always admit one complete event, including oversized legacy
			// records, but do not retain the next rows once this page is full.
			if pageBytes >= schemaBackfillBatchBytes {
				break
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		if len(page) == 0 {
			return nil
		}
		for _, r := range page {
			var event chat.Event
			if err = json.Unmarshal(r.raw, &event); err != nil {
				return fmt.Errorf("decode event %s/%d: %w", r.branch, r.seq, err)
			}
			if r.branch == afterBranch && r.seq <= afterSeq {
				return errors.New("overlapping legacy turn ranges")
			}
			event.Seq = r.seq
			key, mode, visible := transcriptItemIdentity(event, r.seq)
			if r.branch != currentBranch || r.ordinal != currentOrdinal {
				itemStarts = map[string]int64{}
				currentBranch, currentOrdinal = r.branch, r.ordinal
			}
			start := r.seq
			if visible && mode != transcriptItemSingle {
				if prior, ok := itemStarts[key]; ok {
					start = prior
				} else {
					itemStarts[key] = start
				}
			}
			_, err = tx.ExecContext(ctx, `UPDATE events SET turn_ordinal=?,turn_start_seq=?,turn_key=?,item_key=?,item_mode=?,item_start_seq=?,visible=? WHERE branch_id=? AND seq=?`, r.ordinal, r.start, r.turn, key, mode, start, visible, r.branch, r.seq)
			if err != nil {
				return err
			}
			afterBranch, afterSeq = r.branch, r.seq
		}
	}
}

func migrateSessionMaps(ctx context.Context, tx *sql.Tx) error {
	rows, err := tx.QueryContext(ctx, `SELECT c.id,s.provider,s.account_id,s.native_session_id,s.history_through_seq,s.state,
 EXISTS(SELECT 1 FROM runs r WHERE r.harness_session_id=s.id AND r.status IN ('pending','accepted'))
 FROM chats c JOIN harness_sessions s ON s.branch_id=c.active_branch_id
 WHERE s.state<>'retired' ORDER BY c.id,(s.state IN ('ready','legacy_assumed','pending')),s.updated_at,s.id`)
	if err != nil {
		return err
	}
	maps := map[string]map[string]sessionState{}
	for rows.Next() {
		var id, account string
		var provider chat.Provider
		var state sessionState
		var openRun bool
		if err = rows.Scan(&id, &provider, &account, &state.NativeSessionID, &state.HistoryThroughSeq, &state.State, &openRun); err != nil {
			rows.Close()
			return err
		}
		if openRun || (state.State != "ready" && state.State != "legacy_assumed") || state.NativeSessionID == "" {
			state.State = "uncertain"
		}
		if maps[id] == nil {
			maps[id] = map[string]sessionState{}
		}
		maps[id][sessionKey(provider, account)] = state
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for id, states := range maps {
		raw, err := json.Marshal(states)
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, "UPDATE chats SET sessions_json=? WHERE id=?", raw, id); err != nil {
			return err
		}
		var saved []byte
		if err = tx.QueryRowContext(ctx, "SELECT sessions_json FROM chats WHERE id=?", id).Scan(&saved); err != nil {
			return err
		}
		if string(saved) != string(raw) {
			return fmt.Errorf("session map verification failed for chat %s", id)
		}
	}
	return nil
}
