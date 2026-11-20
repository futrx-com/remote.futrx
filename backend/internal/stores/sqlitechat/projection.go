package sqlitechat

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	constants "github.com/futrx-com/remote.futrx.com/internal/config/constants"
	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
)

func putBlobTx(ctx context.Context, tx *sql.Tx, content []byte, mediaType string) (string, error) {
	hash := sha256.Sum256(content)
	id := hex.EncodeToString(hash[:])
	_, err := tx.ExecContext(ctx, "INSERT INTO content_blobs(id,media_type,byte_length,content) VALUES(?,?,?,?) ON CONFLICT(id) DO NOTHING", id, mediaType, len(content), content)
	return id, err
}

func (s *Store) projectEventTx(ctx context.Context, tx *sql.Tx, branch string, ordinal int64, ev servicechat.Event) error {
	key, mode, visible := transcriptItemIdentity(ev, ev.Seq)
	if !visible {
		return nil
	}
	projected, refs := compactTranscriptEvent(servicechat.ID(branch), ev, key+":"+strconv.FormatInt(ev.Seq, 10), 0, 0, ordinal)
	payload := []servicechat.Event{projected}
	start := ev.Seq
	priorSearch := ""
	if mode != transcriptItemSingle {
		var prior []byte
		err := tx.QueryRowContext(ctx, "SELECT start_seq,payload_json,search_text FROM transcript_items WHERE branch_id=? AND turn_ordinal=? AND item_key=? ORDER BY end_seq DESC LIMIT 1", branch, ordinal, key).Scan(&start, &prior, &priorSearch)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if err == nil {
			if mode == transcriptItemAppend {
				// Decode into fresh events: reusing payload would overwrite the
				// current event's shared RawMessage buffers and retain fields that
				// are absent from the previous event.
				var previous []servicechat.Event
				if err = json.Unmarshal(prior, &previous); err != nil {
					return err
				}
				payload = append(previous, projected)
			} else {
				payload[0].Seq = start
			}
		}
	}
	for _, ref := range refs {
		text, err := extractTranscriptContent(ev, ref.fieldKind, ref.fieldKey)
		if err != nil {
			return err
		}
		blobID, err := putBlobTx(ctx, tx, []byte(text), "text/plain; charset=utf-8")
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, "INSERT INTO transcript_content_refs(branch_id,content_id,event_seq,blob_id) VALUES(?,?,?,?)", branch, ref.id, ev.Seq, blobID); err != nil {
			return err
		}
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	// Index full exposed content, including the portion lazily loaded by the
	// UI. A preview-only index would silently omit most large tool results.
	var texts []string
	if mode == transcriptItemAppend && priorSearch != "" {
		texts = append(texts, priorSearch)
	}
	for _, text := range []string{ev.Text, ev.Name, ev.Output, string(ev.Input), ev.Message} {
		if text != "" {
			texts = append(texts, text)
		}
	}
	if ev.Type == "collaboration" && len(ev.Data) > 0 {
		texts = append(texts, string(ev.Data))
	}
	text := strings.Join(texts, "\n")
	result, err := tx.ExecContext(ctx, `INSERT INTO transcript_items(branch_id,turn_ordinal,item_key,start_seq,end_seq,payload_json,payload_bytes,search_text) VALUES(?,?,?,?,?,?,?,?)`, branch, ordinal, key, start, ev.Seq, raw, len(raw), text)
	if err != nil {
		return err
	}
	rowID, err := result.LastInsertId()
	if err != nil {
		return err
	}
	if ev.Type == "assistant_text" || ev.Type == "thinking" {
		return nil
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO transcript_search(rowid,text) VALUES(?,?)", rowID, text)
	return err
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
	out := []branchSegment{}
	for rows.Next() {
		var seg branchSegment
		if err := rows.Scan(&seg.id, &seg.through); err != nil {
			return nil, err
		}
		out = append(out, seg)
	}
	return out, rows.Err()
}

func (s *Store) ReadTranscriptPage(ctx context.Context, id servicechat.ID, q servicechat.TranscriptPageQuery) (servicechat.TranscriptPage, error) {
	branch, last, err := activeBranch(ctx, s.db, id)
	if err != nil {
		return servicechat.TranscriptPage{}, err
	}
	limit := q.Limit
	if limit <= 0 {
		limit = constants.DefaultChatTranscriptTurnLimit
	}
	if limit > constants.MaxChatTranscriptTurnLimit {
		limit = constants.MaxChatTranscriptTurnLimit
	}
	bytes := q.ByteLimit
	if bytes <= 0 {
		bytes = constants.DefaultChatTranscriptByteLimit
	}
	if bytes > constants.MaxChatTranscriptByteLimit {
		bytes = constants.MaxChatTranscriptByteLimit
	}
	before := q.BeforeSeq
	if before <= 0 {
		before = math.MaxInt64
	}
	segments, err := branchSegments(ctx, s.db, branch, last)
	if err != nil {
		return servicechat.TranscriptPage{}, err
	}
	buffer := newTranscriptPageBuffer(limit, bytes)
	for _, segment := range segments {
		rows, err := s.db.QueryContext(ctx, `SELECT i.turn_ordinal,t.source_turn_id,t.start_seq,i.start_seq,i.end_seq,i.payload_json,i.payload_bytes
 FROM transcript_items i JOIN turns t ON t.branch_id=i.branch_id AND t.ordinal=i.turn_ordinal
 WHERE i.branch_id=? AND i.start_seq<? AND i.end_seq<=?
 AND NOT EXISTS(SELECT 1 FROM transcript_items newer WHERE newer.branch_id=i.branch_id AND newer.turn_ordinal=i.turn_ordinal AND newer.item_key=i.item_key AND newer.end_seq>i.end_seq AND newer.end_seq<=?)
 ORDER BY i.start_seq DESC,i.end_seq DESC`, segment.id, before, segment.through, segment.through)
		if err != nil {
			return servicechat.TranscriptPage{}, err
		}
		stop := false
		for rows.Next() {
			var item projectedTranscriptItem
			var raw []byte
			if err = rows.Scan(&item.turnOrdinal, &item.turnID, &item.turnStart, &item.startSeq, &item.endSeq, &raw, &item.payloadBytes); err != nil {
				rows.Close()
				return servicechat.TranscriptPage{}, err
			}
			added, err := buffer.appendItem(item, raw)
			if err != nil {
				rows.Close()
				return servicechat.TranscriptPage{}, err
			}
			if !added {
				stop = true
				break
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return servicechat.TranscriptPage{}, err
		}
		if stop {
			break
		}
	}
	return buffer.page(last), nil
}

func (s *Store) ReadTranscriptContent(ctx context.Context, id servicechat.ID, contentID string, after int64, limit int) (servicechat.TranscriptContentPage, error) {
	if contentID == "" || after < 0 {
		return servicechat.TranscriptContentPage{}, servicechat.ErrTranscriptContentNotFound
	}
	branch, last, err := activeBranch(ctx, s.db, id)
	if err != nil {
		return servicechat.TranscriptContentPage{}, err
	}
	if limit <= 0 {
		limit = constants.DefaultChatTranscriptContentPageBytes
	}
	if limit < utf8.UTFMax {
		limit = utf8.UTFMax
	}
	if limit > constants.MaxChatTranscriptContentPageBytes {
		limit = constants.MaxChatTranscriptContentPageBytes
	}
	var total int64
	var chunk []byte
	// substr on a BLOB uses bytes. Only the requested bounded chunk crosses the
	// SQLite driver, even when the original tool result is hundreds of MB.
	err = s.db.QueryRowContext(ctx, lineageSQL+`SELECT b.byte_length,substr(b.content,?,?)
 FROM lineage l JOIN transcript_content_refs r ON r.branch_id=l.id JOIN content_blobs b ON b.id=r.blob_id
 WHERE r.content_id=? AND r.event_seq<=l.max_seq LIMIT 1`, branch, last, after+1, limit+utf8.UTFMax, contentID).Scan(&total, &chunk)
	if errors.Is(err, sql.ErrNoRows) || after > total && err == nil {
		return servicechat.TranscriptContentPage{}, servicechat.ErrTranscriptContentNotFound
	}
	if err != nil {
		return servicechat.TranscriptContentPage{}, err
	}
	start := 0
	for start < len(chunk) && !utf8.RuneStart(chunk[start]) {
		start++
	}
	end := start + limit
	if end > len(chunk) {
		end = len(chunk)
	}
	for end > start && end < len(chunk) && !utf8.RuneStart(chunk[end]) {
		end--
	}
	complete := after+int64(end) >= total
	var next int64
	if !complete {
		next = after + int64(end)
	}
	return servicechat.TranscriptContentPage{ContentID: contentID, Content: string(chunk[start:end]), NextAfter: next, TotalBytes: total, Complete: complete}, nil
}

func (s *Store) SearchTranscript(ctx context.Context, id servicechat.ID, text string, limit int) ([]servicechat.SearchHit, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return []servicechat.SearchHit{}, nil
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
	if err := s.flushSearch(ctx, id); err != nil {
		return nil, err
	}
	branch, last, err := activeBranch(ctx, s.db, id)
	if err != nil {
		return nil, err
	}
	// Treat user input as a literal phrase. FTS operators and quoting cannot
	// alter scope, and parameter binding keeps SQL out of the query text.
	phrase := "\"" + strings.ReplaceAll(text, "\"", "\"\"") + "\""
	rows, err := s.db.QueryContext(ctx, lineageSQL+`SELECT i.id,t.source_turn_id,i.start_seq,i.end_seq,snippet(transcript_search,0,'','',' … ',32)
 FROM transcript_search JOIN transcript_items i ON i.id=transcript_search.rowid
 JOIN lineage l ON l.id=i.branch_id JOIN turns t ON t.branch_id=i.branch_id AND t.ordinal=i.turn_ordinal
 WHERE transcript_search MATCH ? AND i.end_seq<=l.max_seq
 AND NOT EXISTS(SELECT 1 FROM transcript_items newer WHERE newer.branch_id=i.branch_id AND newer.turn_ordinal=i.turn_ordinal AND newer.item_key=i.item_key AND newer.end_seq>i.end_seq AND newer.end_seq<=l.max_seq)
 ORDER BY rank LIMIT ?`, branch, last, phrase, limit)
	if err != nil {
		return nil, err
	}
	out := []servicechat.SearchHit{}
	for rows.Next() {
		var h servicechat.SearchHit
		var item int64
		if err = rows.Scan(&item, &h.TurnID, &h.StartSeq, &h.EndSeq, &h.Text); err != nil {
			rows.Close()
			return nil, err
		}
		h.ItemID = strconv.FormatInt(item, 10)
		if h.TurnID == "" {
			h.TurnID = "legacy-" + strconv.FormatInt(h.StartSeq, 10)
		}
		out = append(out, h)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	rows, err = s.db.QueryContext(ctx, lineageSQL+`SELECT i.id,i.turn_id,i.start_seq,i.end_seq,snippet(transcript_text_search,0,'','',' … ',32)
 FROM transcript_text_search JOIN transcript_text_runs i ON i.id=transcript_text_search.rowid
 JOIN lineage l ON l.id=i.branch_id
 WHERE transcript_text_search MATCH ? AND i.end_seq<=l.max_seq ORDER BY rank LIMIT ?`, branch, last, phrase, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var h servicechat.SearchHit
		var item int64
		if err = rows.Scan(&item, &h.TurnID, &h.StartSeq, &h.EndSeq, &h.Text); err != nil {
			return nil, err
		}
		h.ItemID = "text-" + strconv.FormatInt(item, 10)
		if h.TurnID == "" {
			h.TurnID = "legacy-" + strconv.FormatInt(h.StartSeq, 10)
		}
		out = append(out, h)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].StartSeq > out[j].StartSeq })
	// Overlap makes boundary-spanning matches possible; do not display the
	// same occurrence twice when it lies entirely in that overlap.
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
