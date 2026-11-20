package sqlitechat

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	chat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
)

// This is the sole derived index. All metadata needed to scope a match lives
// beside its FTS document; no transcript snapshots or search-helper tables.
const searchSchema = `CREATE VIRTUAL TABLE transcript_search USING fts5(
 text,branch_id,turn_id UNINDEXED,start_seq UNINDEXED,end_seq UNINDEXED,
 item_key UNINDEXED,item_mode UNINDEXED,turn_ordinal UNINDEXED,event_type UNINDEXED,parts_json UNINDEXED,
 tokenize='unicode61');`

// Read the current tail and index from one SQLite statement snapshot. Another
// search may have indexed beyond this request's initial tail while it waited.
const searchLineageSQL = `WITH RECURSIVE lineage(id,max_seq) AS (
 SELECT id,last_seq FROM branches WHERE id=?
 UNION ALL
 SELECT b.parent_branch_id,min(l.max_seq,b.fork_seq)
 FROM lineage l JOIN branches b ON b.id=l.id WHERE b.parent_branch_id IS NOT NULL
) `

const searchBatchBytes = 4 * 1024 * 1024
const searchChunkBytes = 8192
const searchOverlapBytes = 2048 // Covers the service's maximum 512-rune query.
type textPart struct {
	Seq  int64  `json:"seq"`
	Text string `json:"text"`
}
type pendingText struct {
	TurnID      string     `json:"turn,omitempty"`
	GroupKey    string     `json:"group,omitempty"`
	TurnOrdinal int64      `json:"ordinal,omitempty"`
	Parts       []textPart `json:"parts,omitempty"`
	HasNew      bool       `json:"new,omitempty"`
	PrefixBuilt bool       `json:"prefixBuilt,omitempty"`
}

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
func flushPending(ctx context.Context, tx *sql.Tx, branch string, p *pendingText, overlap bool) error {
	if p.HasNew {
		if err := insertTextDocument(ctx, tx, branch, *p); err != nil {
			return err
		}
	}
	if overlap {
		p.Parts = suffixParts(p.Parts, searchOverlapBytes)
	} else {
		p.Parts = nil
	}
	p.HasNew = false
	return nil
}
func indexText(ctx context.Context, tx *sql.Tx, branch string, p *pendingText, seq, ordinal int64, turn, group, text string) error {
	if p.GroupKey != group {
		if err := flushPending(ctx, tx, branch, p, false); err != nil {
			return err
		}
		p.GroupKey = group
		p.TurnID = turn
		p.TurnOrdinal = ordinal
	}
	for len(text) > 0 {
		room := searchChunkBytes - partsBytes(p.Parts)
		take := len(text)
		if take > room {
			take = room
			for take > 0 && !utf8.RuneStart(text[take]) {
				take--
			}
		}
		if take == 0 {
			if err := flushPending(ctx, tx, branch, p, true); err != nil {
				return err
			}
			continue
		}
		part := text[:take]
		text = text[take:]
		if n := len(p.Parts); n > 0 && p.Parts[n-1].Seq == seq {
			p.Parts[n-1].Text += part
		} else {
			p.Parts = append(p.Parts, textPart{seq, part})
		}
		p.HasNew = true
		if partsBytes(p.Parts) >= searchChunkBytes-utf8.UTFMax {
			if err := flushPending(ctx, tx, branch, p, true); err != nil {
				return err
			}
		}
	}
	return nil
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
	rows, err := tx.QueryContext(ctx, `SELECT seq,event_type,turn_ordinal,turn_key,visible,
 CASE WHEN event_type IN ('assistant_text','thinking') THEN coalesce(substr(json_extract(payload_json,'$.text'),1,8192),'') ELSE '' END,
 CASE WHEN event_type IN ('assistant_text','thinking') THEN coalesce(length(CAST(json_extract(payload_json,'$.text') AS BLOB)),0) ELSE 0 END,
 CASE WHEN event_type IN ('assistant_text','thinking') THEN json_array(turn_ordinal,event_type,json_extract(payload_json,'$.messageId'),json_extract(payload_json,'$.provider'),json_extract(payload_json,'$.applicationId'),json_extract(payload_json,'$.applicationRequestId'),json_extract(payload_json,'$.scheduledTaskId')) ELSE '' END
 FROM events WHERE branch_id=? AND seq>? AND seq<=? ORDER BY seq LIMIT 128`, seg.id, cursor, seg.through)
	if err != nil {
		return false, err
	}
	type searchEvent struct {
		seq, ordinal, textBytes int64
		typ, turn, text, group  string
		visible                 bool
	}
	batch := make([]searchEvent, 0, 128)
	batchBytes, byteLimitReached := 0, false
	for rows.Next() {
		var e searchEvent
		if err = rows.Scan(&e.seq, &e.typ, &e.ordinal, &e.turn, &e.visible, &e.text, &e.textBytes, &e.group); err != nil {
			rows.Close()
			return false, err
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
		return false, err
	}
	for _, e := range batch {
		if e.typ == "assistant_text" || e.typ == "thinking" {
			if err = indexText(ctx, tx, seg.id, &p, e.seq, e.ordinal, e.turn, e.group, e.text); err != nil {
				return false, err
			}
			// A single assistant event may contain a whole answer. Transfer
			// only bounded UTF-8 chunks when indexing such an event.
			for offset := int64(len(e.text)); offset < e.textBytes; {
				var chunk string
				err = tx.QueryRowContext(ctx, `SELECT substr(CAST(json_extract(payload_json,'$.text') AS BLOB),?,?) FROM events WHERE branch_id=? AND seq=?`, offset+1, searchChunkBytes+utf8.UTFMax, seg.id, e.seq).Scan(&chunk)
				if err != nil {
					return false, err
				}
				chunk = utf8Prefix(chunk, searchChunkBytes)
				if len(chunk) == 0 {
					return false, fmt.Errorf("empty search text chunk at %d", offset)
				}
				if err = indexText(ctx, tx, seg.id, &p, e.seq, e.ordinal, e.turn, e.group, chunk); err != nil {
					return false, err
				}
				offset += int64(len(chunk))
			}
		} else {
			switch e.typ {
			case "user", "tool_use_start", "tool_use_end", "complete", "error", "collaboration", "interaction_request":
				if err = flushPending(ctx, tx, seg.id, &p, false); err != nil {
					return false, err
				}
			}
			if e.visible {
				// Keep full search content inside SQLite, including oversized outputs.
				_, err = tx.ExecContext(ctx, `INSERT INTO transcript_search(text,branch_id,turn_id,start_seq,end_seq,item_key,item_mode,turn_ordinal,event_type,parts_json)
     SELECT coalesce(json_extract(payload_json,'$.text'),'')||char(10)||coalesce(json_extract(payload_json,'$.name'),'')||char(10)||coalesce(json_extract(payload_json,'$.output'),'')||char(10)||coalesce(payload_json -> '$.input','')||char(10)||coalesce(json_extract(payload_json,'$.message'),'')||char(10)||CASE WHEN event_type='collaboration' THEN coalesce(payload_json -> '$.data','') ELSE '' END,
     branch_id,turn_key,seq,seq,item_key,item_mode,turn_ordinal,event_type,'[]' FROM events WHERE branch_id=? AND seq=?`, seg.id, e.seq)
				if err != nil {
					return false, err
				}
			}
		}
		cursor = e.seq
	}
	done := (!byteLimitReached && len(batch) < 128) || cursor >= seg.through
	if done {
		if err = flushPending(ctx, tx, seg.id, &p, true); err != nil {
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

func (s *Store) SearchTranscript(ctx context.Context, id chat.ID, text string, limit int) ([]chat.SearchHit, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return []chat.SearchHit{}, nil
	}
	if len(text) > 4096 {
		return nil, fmt.Errorf("search query exceeds 4096 bytes")
	}
	if limit <= 0 {
		limit = 30
	}
	if limit > 100 {
		limit = 100
	}
	branch, last, err := activeBranch(ctx, s.db, id)
	if err != nil {
		return nil, err
	}
	segments, err := branchSegments(ctx, s.db, branch, last)
	if err != nil {
		return nil, err
	}
	for i := len(segments) - 1; i >= 0; i-- {
		if err = s.catchUpSearch(ctx, segments[i]); err != nil {
			return nil, err
		}
	}
	if err = s.ensureSearchPrefixes(ctx, segments); err != nil {
		return nil, err
	}
	scopes := make([]string, 0, len(segments))
	for _, seg := range segments {
		scopes = append(scopes, ftsPhrase(seg.id))
	}
	phrase := `text : ` + ftsPhrase(text) + ` AND branch_id : (` + strings.Join(scopes, ` OR `) + `)`
	rows, err := s.db.QueryContext(ctx, searchLineageSQL+`SELECT transcript_search.rowid,turn_id,CAST(start_seq AS INTEGER),CAST(end_seq AS INTEGER),snippet(transcript_search,0,'','',' … ',32)
 FROM transcript_search JOIN lineage l ON l.id=transcript_search.branch_id
 WHERE transcript_search MATCH ? AND CAST(end_seq AS INTEGER)<=l.max_seq
 AND (CAST(item_mode AS INTEGER)=0 OR NOT EXISTS(SELECT 1 FROM events newer WHERE newer.branch_id=transcript_search.branch_id AND newer.turn_ordinal=CAST(transcript_search.turn_ordinal AS INTEGER) AND newer.item_key=transcript_search.item_key AND newer.event_type=transcript_search.event_type AND newer.seq>CAST(transcript_search.end_seq AS INTEGER) AND newer.seq<=l.max_seq))
 ORDER BY rank LIMIT ?`, branch, phrase, limit*3)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []chat.SearchHit{}
	for rows.Next() {
		var h chat.SearchHit
		var row int64
		if err = rows.Scan(&row, &h.TurnID, &h.StartSeq, &h.EndSeq, &h.Text); err != nil {
			return nil, err
		}
		h.ItemID = strconv.FormatInt(row, 10)
		if h.TurnID == "" {
			h.TurnID = "legacy-" + strconv.FormatInt(h.StartSeq, 10)
		}
		out = append(out, h)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].StartSeq > out[j].StartSeq })
	unique := out[:0]
	seen := map[string]bool{}
	for _, hit := range out {
		key := hit.TurnID + "\x00" + hit.Text
		if !seen[key] {
			seen[key] = true
			unique = append(unique, hit)
		}
		if len(unique) == limit {
			break
		}
	}
	return unique, nil
}

func ftsPhrase(value string) string { return `"` + strings.ReplaceAll(value, `"`, `""`) + `"` }
