package sqlitechat

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"unicode/utf8"

	constants "github.com/futrx-com/remote.futrx.com/internal/config/constants"
	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
)

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

// Content references identify a field in an authoritative event. They confer no
// access: every read checks the chat's currently retained branch lineage.
type eventContentRef struct {
	Branch string `json:"b"`
	Seq    int64  `json:"s"`
	Kind   string `json:"k"`
	Key    string `json:"f,omitempty"`
}

func contentReference(branch string, seq int64, kind, key string) string {
	raw, _ := json.Marshal(eventContentRef{branch, seq, kind, key})
	return base64.RawURLEncoding.EncodeToString(raw)
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
	budget := q.ByteLimit
	if budget <= 0 {
		budget = constants.DefaultChatTranscriptByteLimit
	}
	if budget > constants.MaxChatTranscriptByteLimit {
		budget = constants.MaxChatTranscriptByteLimit
	}
	before := q.BeforeSeq
	if before <= 0 {
		before = math.MaxInt64
	}
	segments, err := branchSegments(ctx, s.db, branch, last)
	if err != nil {
		return servicechat.TranscriptPage{}, err
	}
	buffer := newTranscriptPageBuffer(limit, budget)
	for _, seg := range segments {
		cursor := before
		stop := false
		for !stop {
			// Close each bounded header cursor before fetching payloads. This
			// avoids exhausting the connection pool under concurrent readers.
			rows, err := s.db.QueryContext(ctx, `SELECT turn_ordinal,turn_key,turn_start_seq,seq,item_key,item_mode,event_type
                FROM events WHERE branch_id=? AND visible=1 AND item_start_seq<? AND seq<=? AND seq=item_start_seq
                ORDER BY item_start_seq DESC,seq DESC LIMIT 64`, seg.id, cursor, seg.through)
			if err != nil {
				return servicechat.TranscriptPage{}, err
			}
			type header struct {
				item           projectedTranscriptItem
				key, eventType string
				mode           transcriptItemMode
			}
			headers := make([]header, 0, 64)
			for rows.Next() {
				var h header
				if err = rows.Scan(&h.item.turnOrdinal, &h.item.turnID, &h.item.turnStart, &h.item.startSeq, &h.key, &h.mode, &h.eventType); err != nil {
					rows.Close()
					return servicechat.TranscriptPage{}, err
				}
				headers = append(headers, h)
			}
			err = rows.Err()
			rows.Close()
			if err != nil {
				return servicechat.TranscriptPage{}, err
			}
			for _, h := range headers {
				item, key, mode, eventType := h.item, h.key, h.mode, h.eventType
				if _, known := buffer.turns[item.turnOrdinal]; !known && len(buffer.turns) == buffer.turnLimit {
					buffer.hasMore = true
					stop = true
					break
				}
				var seqs []int64
				if mode == transcriptItemSingle {
					seqs = []int64{item.startSeq}
				} else {
					types := []string{eventType}
					if mode == transcriptItemAppend {
						if eventType == "tool_use_start" || eventType == "tool_use_end" {
							types = []string{"tool_use_start", "tool_use_end"}
						} else {
							types = []string{"interaction_request", "interaction_resolved"}
						}
					}
					for _, typ := range types {
						var seq int64
						err = s.db.QueryRowContext(ctx, `SELECT seq FROM events WHERE branch_id=? AND turn_ordinal=? AND item_key=? AND event_type=? AND seq<=? ORDER BY seq DESC LIMIT 1`, seg.id, item.turnOrdinal, key, typ, seg.through).Scan(&seq)
						if errors.Is(err, sql.ErrNoRows) {
							continue
						}
						if err != nil {
							return servicechat.TranscriptPage{}, err
						}
						seqs = append(seqs, seq)
					}
				}
				for _, seq := range seqs {
					ev, err := s.readDisplayEvent(ctx, seg.id, seq)
					if err != nil {
						return servicechat.TranscriptPage{}, err
					}
					if seq > item.endSeq {
						item.endSeq = seq
					}
					if mode == transcriptItemReplace {
						ev.Seq = item.startSeq
					}
					item.payload = append(item.payload, ev)
				}
				raw, err := json.Marshal(item.payload)
				if err != nil {
					return servicechat.TranscriptPage{}, err
				}
				item.payloadBytes = len(raw)
				added := buffer.appendItem(item)
				if !added {
					stop = true
					break
				}
			}
			if stop || len(headers) < 64 {
				break
			}
			cursor = headers[len(headers)-1].item.startSeq
		}
		if stop {
			break
		}
	}
	return buffer.page(last), nil
}

// JSON extraction stays inside SQLite. A huge raw/native envelope or tool
// result cannot escape into Go merely to render a small transcript preview.
func (s *Store) readDisplayEvent(ctx context.Context, branch string, seq int64) (servicechat.Event, error) {
	var event servicechat.Event
	var base []byte
	var input, output string
	var inputBytes, outputBytes int64
	err := s.db.QueryRowContext(ctx, `SELECT json_remove(payload_json,'$.raw','$.native','$.input','$.output','$.data'),
 coalesce(substr(payload_json -> '$.input',1,?),''),coalesce(length(CAST(payload_json -> '$.input' AS BLOB)),0),
 coalesce(substr(json_extract(payload_json,'$.output'),1,?),''),coalesce(length(CAST(json_extract(payload_json,'$.output') AS BLOB)),0)
 FROM events WHERE branch_id=? AND seq=?`, constants.ChatTranscriptInlineFieldBytes, constants.ChatTranscriptInlineFieldBytes, branch, seq).Scan(&base, &input, &inputBytes, &output, &outputBytes)
	if err != nil {
		return event, err
	}
	if err = json.Unmarshal(base, &event); err != nil {
		return event, err
	}
	if inputBytes > constants.ChatTranscriptInlineFieldBytes {
		event.Input, _ = json.Marshal(map[string]any{"preview": utf8Prefix(input, constants.ChatTranscriptInlineFieldBytes), "truncated": true, "contentRef": contentReference(branch, seq, "tool_input", "")})
	} else if inputBytes > 0 {
		event.Input = json.RawMessage(input)
	}
	event.Output = utf8Prefix(output, constants.ChatTranscriptInlineFieldBytes)
	if outputBytes > constants.ChatTranscriptInlineFieldBytes {
		event.OutputRef = contentReference(branch, seq, "tool_output", "")
		event.OutputBytes = outputBytes
		event.OutputTruncated = true
	}
	if event.Type == "collaboration" {
		event.Data, err = s.readCollaborationData(ctx, branch, seq)
	} else {
		var data sql.NullString
		err = s.db.QueryRowContext(ctx, `SELECT payload_json -> '$.data' FROM events WHERE branch_id=? AND seq=?`, branch, seq).Scan(&data)
		if data.Valid {
			event.Data = json.RawMessage(data.String)
		}
	}
	return event, err
}

func (s *Store) ReadTranscriptContent(ctx context.Context, id servicechat.ID, contentID string, after int64, limit int) (servicechat.TranscriptContentPage, error) {
	missing := servicechat.ErrTranscriptContentNotFound
	if len(contentID) > 8192 {
		return servicechat.TranscriptContentPage{}, missing
	}
	var ref eventContentRef
	raw, err := base64.RawURLEncoding.DecodeString(contentID)
	if err != nil || len(raw) > 4096 || json.Unmarshal(raw, &ref) != nil || ref.Seq <= 0 || after < 0 {
		return servicechat.TranscriptContentPage{}, missing
	}
	branch, last, err := activeBranch(ctx, s.db, id)
	if err != nil {
		return servicechat.TranscriptContentPage{}, err
	}
	segments, err := branchSegments(ctx, s.db, branch, last)
	if err != nil {
		return servicechat.TranscriptContentPage{}, err
	}
	allowed := false
	for _, seg := range segments {
		if seg.id == ref.Branch && ref.Seq <= seg.through {
			allowed = true
			break
		}
	}
	if !allowed {
		return servicechat.TranscriptContentPage{}, missing
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
	expression, kind, args, err := contentExpression(ref)
	if err != nil {
		return servicechat.TranscriptContentPage{}, err
	}
	args = append(args, ref.Branch, ref.Seq, kind, after+1, limit+utf8.UTFMax)
	var total sql.NullInt64
	var chunk []byte
	err = s.db.QueryRowContext(ctx, `WITH content(value) AS (SELECT `+expression+` FROM events e WHERE branch_id=? AND seq=? AND event_type=?) SELECT length(CAST(value AS BLOB)),substr(CAST(value AS BLOB),?,?) FROM content`, args...).Scan(&total, &chunk)
	if errors.Is(err, sql.ErrNoRows) || err == nil && (!total.Valid || after > total.Int64) {
		return servicechat.TranscriptContentPage{}, missing
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
	complete := after+int64(end) >= total.Int64
	var next int64
	if !complete {
		next = after + int64(end)
	}
	return servicechat.TranscriptContentPage{ContentID: contentID, Content: string(chunk[start:end]), NextAfter: next, TotalBytes: total.Int64, Complete: complete}, nil
}
func contentExpression(ref eventContentRef) (string, string, []any, error) {
	switch ref.Kind {
	case "tool_input":
		return `e.payload_json -> '$.input'`, "tool_use_start", nil, nil
	case "tool_output":
		return `json_extract(e.payload_json,'$.output')`, "tool_use_end", nil, nil
	case "collaboration_tool_input", "collaboration_tool_output":
		index, err := strconv.Atoi(ref.Key)
		if err != nil || index < 0 {
			return "", "", nil, servicechat.ErrTranscriptContentNotFound
		}
		field := `value -> '$.input'`
		if ref.Kind == "collaboration_tool_output" {
			field = `json_extract(value,'$.output')`
		}
		return `(SELECT ` + field + ` FROM json_each(e.payload_json,'$.data.tools') WHERE key=?)`, "collaboration", []any{index}, nil
	case "collaboration_agent_message":
		return `(SELECT json_extract(value,'$.message') FROM json_each(e.payload_json,'$.data.agentsStates') WHERE key=?)`, "collaboration", []any{ref.Key}, nil
	}
	return "", "", nil, fmt.Errorf("%w: unknown field", servicechat.ErrTranscriptContentNotFound)
}
