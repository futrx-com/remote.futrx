package sqlitechat

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
)

func inheritedTurnOrdinal(ctx context.Context, tx *sql.Tx, parent string, cutoff int64) (int64, error) {
	var ordinal int64
	err := tx.QueryRowContext(ctx, lineageSQL+`SELECT e.turn_ordinal FROM lineage l JOIN events e ON e.branch_id=l.id WHERE e.seq<=l.max_seq ORDER BY e.seq DESC LIMIT 1`, parent, cutoff).Scan(&ordinal)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return ordinal, err
}
func (s *Store) ForkChat(ctx context.Context, source servicechat.ID, target servicechat.Meta, cutoff int64) (servicechat.Meta, error) {
	target = normalizeMeta(target)
	if !servicechat.ValidID(target.ID) {
		return servicechat.Meta{}, servicechat.ErrInvalidID
	}
	target.ClearSessionIDs()
	target.ForkPending = false
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return target, err
	}
	defer tx.Rollback()
	parent, last, err := activeBranch(ctx, tx, source)
	if err != nil {
		return target, err
	}
	if cutoff == -1 {
		cutoff = last
	}
	if cutoff < 0 || cutoff > last {
		return target, fmt.Errorf("fork sequence %d is outside history", cutoff)
	}
	branch, err := s.createChatTx(ctx, tx, target, nil)
	if err != nil {
		return target, err
	}
	ordinal, err := inheritedTurnOrdinal(ctx, tx, parent, cutoff)
	if err != nil {
		return target, err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE branches SET parent_branch_id=?,fork_seq=?,last_seq=?,last_turn_ordinal=? WHERE id=?", parent, cutoff, cutoff, ordinal, branch); err != nil {
		return target, err
	}
	return target, tx.Commit()
}

func (s *Store) TruncateEventsBefore(ctx context.Context, id servicechat.ID, before int64) ([]servicechat.Event, error) {
	if err := s.RewindBranch(ctx, id, before); err != nil {
		return nil, err
	}
	return s.ReadEvents(ctx, id)
}

// RewindBranch switches history atomically without materializing the retained
// prefix. HTTP callers can then request the ordinary bounded transcript page.
func (s *Store) RewindBranch(ctx context.Context, id servicechat.ID, before int64) error {
	if before <= 0 {
		return servicechat.ErrInvalidRewindTimestamp
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	parent, last, err := activeBranch(ctx, tx, id)
	if err != nil {
		return err
	}
	var first sql.NullInt64
	err = tx.QueryRowContext(ctx, lineageSQL+`SELECT min(e.seq) FROM lineage l JOIN events e ON e.branch_id=l.id WHERE e.seq<=l.max_seq AND e.t>=?`, parent, last, before).Scan(&first)
	if err != nil {
		return err
	}
	cutoff := last
	if first.Valid {
		cutoff = first.Int64 - 1
	}
	// Respect actual sequence gaps in legacy imports; the bookmark is a valid
	// retained event boundary rather than a number without a corresponding row.
	var retained, lastT int64
	err = tx.QueryRowContext(ctx, lineageSQL+`SELECT coalesce(max(e.seq),0),coalesce(max(e.t),0) FROM lineage l JOIN events e ON e.branch_id=l.id WHERE e.seq<=l.max_seq`, parent, cutoff).Scan(&retained, &lastT)
	if err != nil {
		return err
	}
	cutoff = retained
	ordinal, err := inheritedTurnOrdinal(ctx, tx, parent, cutoff)
	if err != nil {
		return err
	}
	branch := newID()
	_, err = tx.ExecContext(ctx, "INSERT INTO branches(id,chat_id,parent_branch_id,fork_seq,last_seq,last_turn_ordinal) VALUES(?,?,?,?,?,?)", branch, id, parent, cutoff, cutoff, ordinal)
	if err != nil {
		return err
	}
	meta, raw, err := readMeta(ctx, tx, id)
	if err != nil {
		return err
	}
	meta.ClearSessionIDs()
	meta.ForkPending = false
	if lastT == 0 {
		lastT = meta.CreatedAt
	}
	meta.LastMessageAt = lastT
	if err = writeMetaTx(ctx, tx, meta, raw); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE chats SET active_branch_id=?,sessions_json='{}' WHERE id=?", branch, id); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) Delete(ctx context.Context, id servicechat.ID) error {
	if !servicechat.ValidID(id) {
		return servicechat.ErrInvalidID
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// Import checkpoints are bookkeeping, not a reason to retain a deleted chat.
	if _, err = tx.ExecContext(ctx, "DELETE FROM conversation_imports WHERE chat_id=?", id); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM chats WHERE id=?", id); err != nil {
		return err
	}
	// Keep history referenced by a live fork, including ancestors owned by a
	// deleted source chat. Unreferenced history is physically removed.
	const unreachable = `WITH RECURSIVE retained(id) AS (
 SELECT b.id FROM branches b JOIN chats c ON c.id=b.chat_id
 UNION SELECT b.parent_branch_id FROM branches b JOIN retained r ON b.id=r.id WHERE b.parent_branch_id IS NOT NULL
 ) `
	if _, err = tx.ExecContext(ctx, unreachable+`DELETE FROM transcript_search WHERE branch_id NOT IN (SELECT id FROM retained)`); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, unreachable+`DELETE FROM branches WHERE id NOT IN (SELECT id FROM retained)`); err != nil {
		return err
	}
	return tx.Commit()
}

// Verify performs an explicit, potentially expensive maintenance integrity
// check. Ordinary startup checks schema identity without scanning all events.
func (s *Store) Verify(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx, "PRAGMA integrity_check")
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var result string
		if err = rows.Scan(&result); err != nil {
			return err
		}
		if result != "ok" {
			return errors.New(result)
		}
	}
	if err = rows.Err(); err != nil {
		return err
	}
	rows.Close()
	fk, err := s.db.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		return err
	}
	defer fk.Close()
	if fk.Next() {
		return errors.New("conversation foreign key check failed")
	}
	return fk.Err()
}

type branchSegment struct {
	id      string
	through int64
}

func branchSegments(ctx context.Context, q sqlQuerier, branch string, through int64) ([]branchSegment, error) {
	rows, err := q.QueryContext(ctx, `WITH RECURSIVE lineage(id,max_seq,depth) AS (
 SELECT ?,?,0 UNION ALL SELECT b.parent_branch_id,min(l.max_seq,b.fork_seq),depth+1
 FROM lineage l JOIN branches b ON b.id=l.id WHERE b.parent_branch_id IS NOT NULL
 ) SELECT id,max_seq FROM lineage ORDER BY depth`, branch, through)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []branchSegment
	for rows.Next() {
		var seg branchSegment
		if err = rows.Scan(&seg.id, &seg.through); err != nil {
			return nil, err
		}
		out = append(out, seg)
	}
	return out, rows.Err()
}
