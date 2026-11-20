package sqlitechat

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"unicode/utf8"

	constants "github.com/futrx-com/remote.futrx.com/internal/config/constants"
	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
)

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
