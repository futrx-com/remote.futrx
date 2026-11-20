package sqlitechat

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
)

// A branch owns only its appended suffix. Each ancestor contributes no farther
// than the child's fork point, even if that ancestor later receives new events.
const lineageSQL = `WITH RECURSIVE lineage(id,max_seq) AS (
 SELECT ?,?
 UNION ALL
 SELECT b.parent_branch_id,min(l.max_seq,b.fork_seq)
 FROM lineage l JOIN branches b ON b.id=l.id WHERE b.parent_branch_id IS NOT NULL
) `

func readEventsRange(ctx context.Context, q sqlQuerier, branch string, after, through int64) ([]servicechat.Event, error) {
	segments, err := branchSegments(ctx, q, branch, through)
	if err != nil {
		return nil, err
	}
	out := []servicechat.Event{}
	for i := len(segments) - 1; i >= 0; i-- {
		seg := segments[i]
		rows, err := q.QueryContext(ctx, `SELECT seq,payload_json FROM events WHERE branch_id=? AND seq>? AND seq<=? ORDER BY seq`, seg.id, after, seg.through)
		if err != nil {
			return nil, err
		}
		events, err := decodeEventRows(rows)
		rows.Close()
		if err != nil {
			return nil, err
		}
		out = append(out, events...)
	}
	return out, nil
}
func decodeEventRows(rows *sql.Rows) ([]servicechat.Event, error) {
	out := []servicechat.Event{}
	for rows.Next() {
		var seq int64
		var raw []byte
		if err := rows.Scan(&seq, &raw); err != nil {
			return nil, err
		}
		var ev servicechat.Event
		if err := json.Unmarshal(raw, &ev); err != nil {
			return nil, fmt.Errorf("decode authoritative event %d: %w", seq, err)
		}
		ev.Seq = seq
		ev.NormalizeSession()
		out = append(out, ev)
	}
	return out, rows.Err()
}
func (s *Store) ReadEvents(ctx context.Context, id servicechat.ID) ([]servicechat.Event, error) {
	return s.ReadEventsAfter(ctx, id, 0)
}
func (s *Store) ReadEventsAfter(ctx context.Context, id servicechat.ID, after int64) ([]servicechat.Event, error) {
	branch, last, err := activeBranch(ctx, s.db, id)
	if err != nil {
		return nil, err
	}
	return readEventsRange(ctx, s.db, branch, after, last)
}
func (s *Store) ScanEvents(ctx context.Context, id servicechat.ID, visit func(servicechat.Event)) error {
	branch, last, err := activeBranch(ctx, s.db, id)
	if err != nil {
		return err
	}
	// Bounded range pages keep the connection free while the caller processes
	// each event; a stable captured branch/tail makes concurrent appends harmless.
	segments, err := branchSegments(ctx, s.db, branch, last)
	if err != nil {
		return err
	}
	for i := len(segments) - 1; i >= 0; i-- {
		seg := segments[i]
		var after int64
		for after < seg.through {
			rows, err := s.db.QueryContext(ctx, `SELECT seq,payload_json FROM events WHERE branch_id=? AND seq>? AND seq<=? ORDER BY seq LIMIT 256`, seg.id, after, seg.through)
			if err != nil {
				return err
			}
			events, err := decodeEventRows(rows)
			rows.Close()
			if err != nil {
				return err
			}
			if len(events) == 0 {
				break
			}
			for _, ev := range events {
				visit(ev)
				after = ev.Seq
			}
		}
	}
	return ctx.Err()
}
func (s *Store) ReadEventsPage(ctx context.Context, id servicechat.ID, q servicechat.EventPageQuery) (servicechat.EventPage, error) {
	branch, last, err := activeBranch(ctx, s.db, id)
	if err != nil {
		return servicechat.EventPage{}, err
	}
	limit := q.Limit
	if limit <= 0 {
		limit = 200
	}
	if limit > 1000 {
		limit = 1000
	}
	before := q.BeforeSeq
	if before <= 0 {
		before = math.MaxInt64
	}
	segments, err := branchSegments(ctx, s.db, branch, last)
	if err != nil {
		return servicechat.EventPage{}, err
	}
	events := []servicechat.Event{}
	for _, seg := range segments {
		rows, err := s.db.QueryContext(ctx, `SELECT seq,payload_json FROM events WHERE branch_id=? AND seq<=? AND seq<? ORDER BY seq DESC LIMIT ?`, seg.id, seg.through, before, limit+1-len(events))
		if err != nil {
			return servicechat.EventPage{}, err
		}
		batch, err := decodeEventRows(rows)
		rows.Close()
		if err != nil {
			return servicechat.EventPage{}, err
		}
		events = append(events, batch...)
		if len(events) > limit {
			break
		}
	}
	more := len(events) > limit
	if more {
		events = events[:limit]
	}
	for i, j := 0, len(events)-1; i < j; i, j = i+1, j-1 {
		events[i], events[j] = events[j], events[i]
	}
	var next int64
	if more {
		next = events[0].Seq
	}
	return servicechat.EventPage{Events: events, LastSeq: last, HasMore: more, NextBefore: next}, nil
}
func (s *Store) AppendEvent(ctx context.Context, id servicechat.ID, ev servicechat.Event) (servicechat.Event, error) {
	return s.append(ctx, id, ev, true)
}
func (s *Store) AppendCopiedEvent(ctx context.Context, id servicechat.ID, ev servicechat.Event) (servicechat.Event, error) {
	return s.append(ctx, id, ev, false)
}
func (s *Store) append(ctx context.Context, id servicechat.ID, ev servicechat.Event, touch bool) (servicechat.Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ev, err
	}
	defer tx.Rollback()
	branch, _, err := activeBranch(ctx, tx, id)
	if err != nil {
		return ev, err
	}
	ev.Seq = 0
	if ev.T == 0 {
		ev.T = time.Now().UnixMilli()
	}
	ev, err = s.appendEventTx(ctx, tx, branch, ev, nil)
	if err != nil {
		return ev, err
	}
	if touch && (ev.Type == "user" || ev.Type == "complete" || ev.Type == "error") {
		meta, raw, err := readMeta(ctx, tx, id)
		if err != nil {
			return ev, err
		}
		meta.LastMessageAt = ev.T
		if err = writeMetaTx(ctx, tx, meta, raw); err != nil {
			return ev, err
		}
	}
	return ev, tx.Commit()
}

// appendEventTx deliberately does not mutate chat metadata: import must retain
// the exact legacy metadata snapshot, and copied history is not new activity.
func (s *Store) appendEventTx(ctx context.Context, tx *sql.Tx, branch string, ev servicechat.Event, raw []byte) (servicechat.Event, error) {
	var last, ordinal, start int64
	var turnID string
	var hasUser bool
	if err := tx.QueryRowContext(ctx, "SELECT last_seq,last_turn_ordinal,last_turn_id,last_turn_start_seq,last_turn_has_user FROM branches WHERE id=?", branch).Scan(&last, &ordinal, &turnID, &start, &hasUser); err != nil {
		return ev, err
	}
	if ev.Seq == 0 {
		ev.Seq = last + 1
	}
	if ev.Seq <= last {
		return ev, fmt.Errorf("event sequence %d does not follow %d", ev.Seq, last)
	}
	ev.NormalizeSession()
	payload, err := json.Marshal(ev)
	if err != nil {
		return ev, err
	}
	fresh := start == 0 || servicechat.EventStartsTranscriptTurn(turnID, hasUser, true, ev)
	if fresh {
		ordinal++
		turnID = ev.TurnID
		hasUser = ev.Type == "user"
		start = ev.Seq
	} else {
		if turnID == "" && ev.TurnID != "" {
			turnID = ev.TurnID
			// Legacy streams can reveal their turn identifier after the first
			// event. Only update the index fields; original payloads stay intact.
			if _, err = tx.ExecContext(ctx, "UPDATE events SET turn_key=? WHERE branch_id=? AND turn_ordinal=?", turnID, branch, ordinal); err != nil {
				return ev, err
			}
		}
		hasUser = hasUser || ev.Type == "user"
	}
	key, mode, visible := transcriptItemIdentity(ev, ev.Seq)
	itemStart := ev.Seq
	if visible && mode != transcriptItemSingle {
		err = tx.QueryRowContext(ctx, "SELECT item_start_seq FROM events WHERE branch_id=? AND turn_ordinal=? AND item_key=? ORDER BY seq DESC LIMIT 1", branch, ordinal, key).Scan(&itemStart)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return ev, err
		}
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO events
	 (branch_id,seq,t,event_type,turn_id,payload_json,raw_json,turn_ordinal,turn_start_seq,turn_key,item_key,item_mode,item_start_seq,visible)
	 VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, branch, ev.Seq, ev.T, ev.Type, ev.TurnID, payload, raw, ordinal, start, turnID, key, mode, itemStart, visible); err != nil {
		return ev, err
	}
	_, err = tx.ExecContext(ctx, "UPDATE branches SET last_seq=?,last_turn_ordinal=?,last_turn_id=?,last_turn_start_seq=?,last_turn_has_user=? WHERE id=?", ev.Seq, ordinal, turnID, start, hasUser, branch)
	return ev, err
}

func (s *Store) ReadTranscriptEventWindow(ctx context.Context, id servicechat.ID, before int64, limit int) (servicechat.TranscriptEventWindow, error) {
	branch, last, err := activeBranch(ctx, s.db, id)
	if err != nil {
		return servicechat.TranscriptEventWindow{}, err
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	cutoff := last
	if before > 0 && before-1 < cutoff {
		cutoff = before - 1
	}
	rows, err := s.db.QueryContext(ctx, lineageSQL+`SELECT e.seq FROM lineage l JOIN events e ON e.branch_id=l.id WHERE e.seq=e.turn_start_seq AND e.seq<=l.max_seq ORDER BY e.seq DESC LIMIT ?`, branch, cutoff, limit+1)
	if err != nil {
		return servicechat.TranscriptEventWindow{}, err
	}
	var start int64
	for rows.Next() {
		if err = rows.Scan(&start); err != nil {
			rows.Close()
			return servicechat.TranscriptEventWindow{}, err
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return servicechat.TranscriptEventWindow{}, err
	}
	events, err := readEventsRange(ctx, s.db, branch, start-1, cutoff)
	return servicechat.TranscriptEventWindow{Events: events, LastSeq: last}, err
}
