package sqlitechat

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"

	chat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
)

// Read the current tail and index from one SQLite statement snapshot. Another
// search may have indexed beyond this request's initial tail while it waited.
const searchLineageSQL = `WITH RECURSIVE lineage(id,max_seq) AS (
 SELECT id,last_seq FROM branches WHERE id=?
 UNION ALL
 SELECT b.parent_branch_id,min(l.max_seq,b.fork_seq)
 FROM lineage l JOIN branches b ON b.id=l.id WHERE b.parent_branch_id IS NOT NULL
) `

// The start event supplies the hit's time and its real type: grouped
// thinking and answer text share one index label.
const searchEventTimeSQL = `coalesce((SELECT source.t FROM events source WHERE source.branch_id=transcript_search.branch_id AND source.seq=CAST(transcript_search.start_seq AS INTEGER)),0)`

const searchMatchesSQL = searchLineageSQL + `SELECT transcript_search.rowid,turn_id,CAST(start_seq AS INTEGER),CAST(end_seq AS INTEGER),snippet(transcript_search,0,'` + snippetMatchOpen + `','` + snippetMatchClose + `',' … ',32),
 coalesce((SELECT source.event_type FROM events source WHERE source.branch_id=transcript_search.branch_id AND source.seq=CAST(transcript_search.start_seq AS INTEGER)),transcript_search.event_type),
 ` + searchEventTimeSQL + `
 FROM transcript_search JOIN lineage l ON l.id=transcript_search.branch_id
 WHERE transcript_search MATCH ? AND CAST(end_seq AS INTEGER)<=l.max_seq AND (?=0 OR CAST(start_seq AS INTEGER)<?)
 AND (?=0 OR ` + searchEventTimeSQL + `<=?)
 AND (CAST(item_mode AS INTEGER)=0 OR NOT EXISTS(SELECT 1 FROM events newer WHERE newer.branch_id=transcript_search.branch_id AND newer.turn_ordinal=CAST(transcript_search.turn_ordinal AS INTEGER) AND newer.item_key=transcript_search.item_key AND newer.event_type=transcript_search.event_type AND newer.seq>CAST(transcript_search.end_seq AS INTEGER) AND newer.seq<=l.max_seq))
 ORDER BY CAST(start_seq AS INTEGER) DESC LIMIT ?`

func (s *Store) SearchTranscript(ctx context.Context, id chat.ID, text string, limit int) ([]chat.SearchHit, error) {
	return s.SearchTranscriptPage(ctx, id, text, limit, 0)
}

// SearchTranscriptPage returns matches newest first. A positive `before`
// continues a previous page: only matches starting below that sequence.
func (s *Store) SearchTranscriptPage(ctx context.Context, id chat.ID, text string, limit int, before int64) ([]chat.SearchHit, error) {
	return s.searchTranscript(ctx, id, text, limit, before, 0)
}

// SearchTranscriptUntil returns matches newest first, recorded at or before
// maxT (Unix milliseconds) when maxT is positive. Cross-chat search pages by
// time, since sequences are per chat.
func (s *Store) SearchTranscriptUntil(ctx context.Context, id chat.ID, text string, limit int, maxT int64) ([]chat.SearchHit, error) {
	return s.searchTranscript(ctx, id, text, limit, 0, maxT)
}

func (s *Store) searchTranscript(ctx context.Context, id chat.ID, text string, limit int, before, maxT int64) ([]chat.SearchHit, error) {
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
	// The query's last word matches as a prefix, so results follow typing:
	// "t" finds "test" and "turn", not only a standalone "t".
	phrase := `text : ` + ftsPhrase(text) + `* AND branch_id : (` + strings.Join(scopes, ` OR `) + `)`
	rows, err := s.db.QueryContext(ctx, searchMatchesSQL, branch, phrase, before, before, maxT, maxT, limit*3)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return readSearchHits(rows, limit)
}

func readSearchHits(rows *sql.Rows, limit int) ([]chat.SearchHit, error) {
	out := []chat.SearchHit{}
	for rows.Next() {
		var h chat.SearchHit
		var row int64
		if err := rows.Scan(&row, &h.TurnID, &h.StartSeq, &h.EndSeq, &h.Text, &h.Kind, &h.T); err != nil {
			return nil, err
		}
		h.Text, h.Highlights = searchSnippet(h.Text)
		h.ItemID = strconv.FormatInt(row, 10)
		if h.TurnID == "" {
			h.TurnID = "legacy-" + strconv.FormatInt(h.StartSeq, 10)
		}
		out = append(out, h)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
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
