package sqlitechat

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"unicode/utf8"
)

// This is the sole derived index. All metadata needed to scope a match lives
// beside its FTS document; no transcript snapshots or search-helper tables.
const searchSchema = `CREATE VIRTUAL TABLE transcript_search USING fts5(
 text,branch_id,turn_id UNINDEXED,start_seq UNINDEXED,end_seq UNINDEXED,
 item_key UNINDEXED,item_mode UNINDEXED,turn_ordinal UNINDEXED,event_type UNINDEXED,parts_json UNINDEXED,
 tokenize='unicode61');`

const searchBatchBytes = 4 * 1024 * 1024

func readPendingText(ctx context.Context, tx *sql.Tx, branch string) (pendingText, error) {
	var p pendingText
	var raw []byte
	err := tx.QueryRowContext(ctx, `SELECT search_pending_json FROM branches WHERE id=?`, branch).Scan(&raw)
	if err == nil {
		err = json.Unmarshal(raw, &p)
	}
	return p, err
}

func writePendingText(ctx context.Context, tx *sql.Tx, branch string, p pendingText) error {
	raw, err := json.Marshal(p)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE branches SET search_pending_json=? WHERE id=?`, raw, branch)
	return err
}

func insertTextDocument(ctx context.Context, tx *sql.Tx, branch string, p pendingText) error {
	if len(p.Parts) == 0 || partsBytes(p.Parts) == 0 {
		return nil
	}
	raw, err := json.Marshal(p.Parts)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO transcript_search(text,branch_id,turn_id,start_seq,end_seq,item_key,item_mode,turn_ordinal,event_type,parts_json) VALUES(?,?,?,?,?,?,0,?,'assistant_text',?)`, partsText(p.Parts), branch, p.TurnID, p.Parts[0].Seq, p.Parts[len(p.Parts)-1].Seq, p.GroupKey, p.TurnOrdinal, raw)
	return err
}

// Catch-up runs only when search is requested, in bounded event batches. A
// failed derived index never rolls back a conversation append. Its cursor is
// committed with each batch, making retries idempotent after interruption.
func (s *Store) catchUpSearch(ctx context.Context, seg branchSegment) error {
	for {
		done, err := s.searchBatch(ctx, seg)
		if err != nil || done {
			return err
		}
	}
}

func (s *Store) searchBatch(ctx context.Context, seg branchSegment) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var cursor int64
	err = tx.QueryRowContext(ctx, `SELECT search_seq FROM branches WHERE id=?`, seg.id).Scan(&cursor)
	if err != nil {
		return false, err
	}
	if cursor >= seg.through {
		return true, nil
	}
	p, err := readPendingText(ctx, tx, seg.id)
	if err != nil {
		return false, err
	}
	batch, byteLimitReached, err := readSearchBatch(ctx, tx, seg, cursor)
	if err != nil {
		return false, err
	}
	emit := func(pending pendingText) error {
		return insertTextDocument(ctx, tx, seg.id, pending)
	}
	for _, e := range batch {
		if err = indexSearchEvent(ctx, tx, seg.id, &p, e, emit); err != nil {
			return false, err
		}
		cursor = e.seq
	}
	done := (!byteLimitReached && len(batch) < 128) || cursor >= seg.through
	if done {
		if err = p.flush(true, emit); err != nil {
			return false, err
		}
		cursor = seg.through
	}
	if err = writePendingText(ctx, tx, seg.id, p); err != nil {
		return false, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE branches SET search_seq=? WHERE id=?`, cursor, seg.id); err != nil {
		return false, err
	}
	return done, tx.Commit()
}

type searchEvent struct {
	seq, ordinal, textBytes int64
	typ, turn, text, group  string
	visible                 bool
}

func readSearchBatch(ctx context.Context, tx *sql.Tx, seg branchSegment, cursor int64) ([]searchEvent, bool, error) {
	rows, err := tx.QueryContext(ctx, `SELECT seq,event_type,turn_ordinal,turn_key,visible,
 CASE WHEN event_type IN ('assistant_text','thinking') THEN coalesce(substr(json_extract(payload_json,'$.text'),1,8192),'') ELSE '' END,
 CASE WHEN event_type IN ('assistant_text','thinking') THEN coalesce(length(CAST(json_extract(payload_json,'$.text') AS BLOB)),0) ELSE 0 END,
 CASE WHEN event_type IN ('assistant_text','thinking') THEN json_array(turn_ordinal,event_type,json_extract(payload_json,'$.messageId'),json_extract(payload_json,'$.provider'),json_extract(payload_json,'$.applicationId'),json_extract(payload_json,'$.applicationRequestId'),json_extract(payload_json,'$.scheduledTaskId')) ELSE '' END
 FROM events WHERE branch_id=? AND seq>? AND seq<=? ORDER BY seq LIMIT 128`, seg.id, cursor, seg.through)
	if err != nil {
		return nil, false, err
	}
	batch := make([]searchEvent, 0, 128)
	batchBytes, byteLimitReached := 0, false
	for rows.Next() {
		var e searchEvent
		if err = rows.Scan(&e.seq, &e.typ, &e.ordinal, &e.turn, &e.visible, &e.text, &e.textBytes, &e.group); err != nil {
			rows.Close()
			return nil, false, err
		}
		batch = append(batch, e)
		batchBytes += len(e.typ) + len(e.turn) + len(e.text) + len(e.group)
		if batchBytes >= searchBatchBytes {
			byteLimitReached = true
			break
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, false, err
	}
	return batch, byteLimitReached, nil
}

func indexSearchEvent(ctx context.Context, tx *sql.Tx, branch string, p *pendingText, e searchEvent, emit func(pendingText) error) error {
	if e.typ == "assistant_text" || e.typ == "thinking" {
		if err := p.appendText(e.seq, e.ordinal, e.turn, e.group, e.text, emit); err != nil {
			return err
		}
		// A single assistant event may contain a whole answer. Transfer
		// only bounded UTF-8 chunks when indexing such an event.
		for offset := int64(len(e.text)); offset < e.textBytes; {
			var chunk string
			err := tx.QueryRowContext(ctx, `SELECT substr(CAST(json_extract(payload_json,'$.text') AS BLOB),?,?) FROM events WHERE branch_id=? AND seq=?`, offset+1, searchChunkBytes+utf8.UTFMax, branch, e.seq).Scan(&chunk)
			if err != nil {
				return err
			}
			chunk = utf8Prefix(chunk, searchChunkBytes)
			if len(chunk) == 0 {
				return fmt.Errorf("empty search text chunk at %d", offset)
			}
			if err := p.appendText(e.seq, e.ordinal, e.turn, e.group, chunk, emit); err != nil {
				return err
			}
			offset += int64(len(chunk))
		}
	} else {
		switch e.typ {
		case "user", "tool_use_start", "tool_use_end", "complete", "error", "collaboration", "interaction_request":
			if err := p.flush(false, emit); err != nil {
				return err
			}
		}
		if e.visible {
			// Keep full search content inside SQLite, including oversized outputs.
			_, err := tx.ExecContext(ctx, `INSERT INTO transcript_search(text,branch_id,turn_id,start_seq,end_seq,item_key,item_mode,turn_ordinal,event_type,parts_json)
     SELECT coalesce(json_extract(payload_json,'$.text'),'')||char(10)||coalesce(json_extract(payload_json,'$.name'),'')||char(10)||coalesce(json_extract(payload_json,'$.output'),'')||char(10)||coalesce(payload_json -> '$.input','')||char(10)||coalesce(json_extract(payload_json,'$.message'),'')||char(10)||CASE WHEN event_type='collaboration' THEN coalesce(payload_json -> '$.data','') ELSE '' END,
     branch_id,turn_key,seq,seq,item_key,item_mode,turn_ordinal,event_type,'[]' FROM events WHERE branch_id=? AND seq=?`, branch, e.seq)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Store) ensureSearchPrefixes(ctx context.Context, segments []branchSegment) error {
	if len(segments) < 2 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	child := segments[0].id
	state, err := readPendingText(ctx, tx, child)
	if err != nil {
		return err
	}
	if state.PrefixBuilt {
		return nil
	}
	for _, seg := range segments[1:] {
		rows, err := tx.QueryContext(ctx, `SELECT turn_id,turn_ordinal,item_key,parts_json FROM transcript_search WHERE transcript_search MATCH ? AND branch_id=? AND CAST(start_seq AS INTEGER)<=? AND CAST(end_seq AS INTEGER)>? AND parts_json!='[]'`, `branch_id : `+ftsPhrase(seg.id), seg.id, seg.through, seg.through)
		if err != nil {
			return err
		}
		var partials []pendingText
		for rows.Next() {
			var p pendingText
			var raw []byte
			if err = rows.Scan(&p.TurnID, &p.TurnOrdinal, &p.GroupKey, &raw); err != nil {
				rows.Close()
				return err
			}
			if err = json.Unmarshal(raw, &p.Parts); err != nil {
				rows.Close()
				return err
			}
			kept := p.Parts[:0]
			for _, part := range p.Parts {
				if part.Seq <= seg.through {
					kept = append(kept, part)
				}
			}
			p.Parts = kept
			if len(kept) > 0 {
				partials = append(partials, p)
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		for _, p := range partials {
			if err = insertTextDocument(ctx, tx, child, p); err != nil {
				return err
			}
		}
	}
	state.PrefixBuilt = true
	if err = writePendingText(ctx, tx, child, state); err != nil {
		return err
	}
	return tx.Commit()
}
