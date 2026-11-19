package sqlitechat

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"unicode/utf8"

	chat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
)

const searchChunkBytes = 8192
const searchOverlapBytes = 2048 // Covers the service's maximum 512-rune query.

type textPart struct {
	Seq  int64  `json:"seq"`
	Text string `json:"text"`
}
type pendingText struct {
	turnID, groupKey string
	parts            []textPart
	hasNew           bool
}

func readPendingText(ctx context.Context, tx *sql.Tx, branch string) (pendingText, error) {
	var p pendingText
	var raw []byte
	err := tx.QueryRowContext(ctx, "SELECT turn_id,group_key,parts_json,has_new FROM transcript_text_pending WHERE branch_id=?", branch).Scan(&p.turnID, &p.groupKey, &raw, &p.hasNew)
	if errors.Is(err, sql.ErrNoRows) {
		return p, nil
	}
	if err != nil {
		return p, err
	}
	err = json.Unmarshal(raw, &p.parts)
	return p, err
}
func writePendingText(ctx context.Context, tx *sql.Tx, branch string, p pendingText) error {
	if len(p.parts) == 0 {
		_, err := tx.ExecContext(ctx, "DELETE FROM transcript_text_pending WHERE branch_id=?", branch)
		return err
	}
	raw, err := json.Marshal(p.parts)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO transcript_text_pending(branch_id,turn_id,group_key,parts_json,has_new) VALUES(?,?,?,?,?) ON CONFLICT(branch_id) DO UPDATE SET turn_id=excluded.turn_id,group_key=excluded.group_key,parts_json=excluded.parts_json,has_new=excluded.has_new`, branch, p.turnID, p.groupKey, raw, p.hasNew)
	return err
}
func partsText(parts []textPart) string {
	var b strings.Builder
	for _, p := range parts {
		b.WriteString(p.Text)
	}
	return b.String()
}
func partsBytes(parts []textPart) int {
	size := 0
	for _, p := range parts {
		size += len(p.Text)
	}
	return size
}

func suffixParts(parts []textPart, limit int) []textPart {
	keep := []textPart{}
	used := 0
	for i := len(parts) - 1; i >= 0 && used < limit; i-- {
		p := parts[i]
		room := limit - used
		if len(p.Text) > room {
			start := len(p.Text) - room
			for start > 0 && !utf8.RuneStart(p.Text[start]) {
				start--
			}
			p.Text = p.Text[start:]
		}
		keep = append(keep, p)
		used += len(p.Text)
	}
	for i, j := 0, len(keep)-1; i < j; i, j = i+1, j-1 {
		keep[i], keep[j] = keep[j], keep[i]
	}
	return keep
}

func insertTextDocument(ctx context.Context, tx *sql.Tx, branch string, p pendingText) error {
	if len(p.parts) == 0 || partsBytes(p.parts) == 0 {
		return nil
	}
	raw, err := json.Marshal(p.parts)
	if err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO transcript_text_runs(branch_id,turn_id,group_key,start_seq,end_seq,parts_json) VALUES(?,?,?,?,?,?)`, branch, p.turnID, p.groupKey, p.parts[0].Seq, p.parts[len(p.parts)-1].Seq, raw)
	if err != nil {
		return err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO transcript_text_search(rowid,text) VALUES(?,?)", id, partsText(p.parts))
	return err
}
func flushTextPendingTx(ctx context.Context, tx *sql.Tx, branch string, keepOverlap bool) error {
	p, err := readPendingText(ctx, tx, branch)
	if err != nil {
		return err
	}
	if p.hasNew {
		if err = insertTextDocument(ctx, tx, branch, p); err != nil {
			return err
		}
	}
	if keepOverlap {
		p.parts = suffixParts(p.parts, searchOverlapBytes)
		p.hasNew = false
	} else {
		p.parts = nil
	}
	return writePendingText(ctx, tx, branch, p)
}

func indexTextEventTx(ctx context.Context, tx *sql.Tx, branch string, turnOrdinal int64, event chat.Event) error {
	if event.Type != "assistant_text" && event.Type != "thinking" {
		switch event.Type {
		case "user", "tool_use_start", "tool_use_end", "complete", "error", "collaboration", "interaction_request":
			return flushTextPendingTx(ctx, tx, branch, false)
		}
		return nil
	}
	if event.Text == "" {
		return nil
	}
	identity, _ := json.Marshal([]string{strconv.FormatInt(turnOrdinal, 10), event.Type, event.MessageID, string(event.Provider), event.ApplicationID, event.ApplicationRequestID, event.ScheduledTaskID})
	p, err := readPendingText(ctx, tx, branch)
	if err != nil {
		return err
	}
	if p.groupKey != string(identity) {
		if p.hasNew {
			if err = insertTextDocument(ctx, tx, branch, p); err != nil {
				return err
			}
		}
		p = pendingText{turnID: event.TurnID, groupKey: string(identity)}
	}
	remaining := event.Text
	for len(remaining) > 0 {
		room := searchChunkBytes - partsBytes(p.parts)
		take := len(remaining)
		if take > room {
			take = room
			for take > 0 && !utf8.RuneStart(remaining[take]) {
				take--
			}
		}
		if take == 0 {
			if p.hasNew {
				if err = insertTextDocument(ctx, tx, branch, p); err != nil {
					return err
				}
			}
			p.parts = suffixParts(p.parts, searchOverlapBytes)
			p.hasNew = false
			continue
		}
		text := remaining[:take]
		remaining = remaining[take:]
		if n := len(p.parts); n > 0 && p.parts[n-1].Seq == event.Seq {
			p.parts[n-1].Text += text
		} else {
			p.parts = append(p.parts, textPart{event.Seq, text})
		}
		p.hasNew = true
		if partsBytes(p.parts) >= searchChunkBytes-utf8.UTFMax {
			if err = insertTextDocument(ctx, tx, branch, p); err != nil {
				return err
			}
			p.parts = suffixParts(p.parts, searchOverlapBytes)
			p.hasNew = false
		}
	}
	return writePendingText(ctx, tx, branch, p)
}

// Only the at-most-one chunk crossing each ancestor cutoff needs a derived
// prefix snapshot. Filtering seq-tagged parts prevents future text from being
// searchable after a rewind/fork, without copying the canonical conversation.
func materializeSearchPrefixTx(ctx context.Context, tx *sql.Tx, child, parent string, cutoff int64) error {
	if err := flushTextPendingTx(ctx, tx, parent, true); err != nil {
		return err
	}
	segments, err := branchSegments(ctx, tx, parent, cutoff)
	if err != nil {
		return err
	}
	var partials []pendingText
	for _, seg := range segments {
		rows, err := tx.QueryContext(ctx, `SELECT turn_id,group_key,parts_json FROM transcript_text_runs WHERE branch_id=? AND start_seq<=? AND end_seq>?`, seg.id, seg.through, seg.through)
		if err != nil {
			return err
		}
		for rows.Next() {
			var p pendingText
			var raw []byte
			if err = rows.Scan(&p.turnID, &p.groupKey, &raw); err != nil {
				rows.Close()
				return err
			}
			if err = json.Unmarshal(raw, &p.parts); err != nil {
				rows.Close()
				return err
			}
			kept := p.parts[:0]
			for _, part := range p.parts {
				if part.Seq <= seg.through {
					kept = append(kept, part)
				}
			}
			p.parts = kept
			if len(kept) > 0 {
				partials = append(partials, p)
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
	}
	for _, p := range partials {
		if err = insertTextDocument(ctx, tx, child, p); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) flushSearch(ctx context.Context, id chat.ID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	branch, _, err := activeBranch(ctx, tx, id)
	if err != nil {
		return err
	}
	if err = flushTextPendingTx(ctx, tx, branch, true); err != nil {
		return err
	}
	return tx.Commit()
}
