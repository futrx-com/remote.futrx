package sqlitechat

import (
	"context"
	"database/sql"
	"encoding/json"
	"strconv"

	constants "github.com/futrx-com/remote.futrx.com/internal/config/constants"
	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
)

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

func (s *Store) readCollaborationData(ctx context.Context, branch string, seq int64) (json.RawMessage, error) {
	var raw sql.NullString
	var hasTools, hasStates bool
	err := s.db.QueryRowContext(ctx, `WITH source AS (SELECT payload_json -> '$.data' AS raw FROM events WHERE branch_id=? AND seq=?),
 base AS (SELECT raw,CASE WHEN json_type(raw,'$.tools')='array' THEN json_remove(raw,'$.tools') ELSE raw END AS value FROM source)
 SELECT CASE WHEN json_type(raw,'$.agentsStates')='object' THEN json_remove(value,'$.agentsStates') ELSE value END,
 coalesce(json_type(raw,'$.tools')='array',0),coalesce(json_type(raw,'$.agentsStates')='object',0) FROM base`, branch, seq).Scan(&raw, &hasTools, &hasStates)
	if err != nil {
		return nil, err
	}
	if !raw.Valid {
		return nil, nil
	}
	var value any
	if err = json.Unmarshal([]byte(raw.String), &value); err != nil {
		return nil, err
	}
	data, isObject := value.(map[string]any)
	if !isObject {
		return json.RawMessage(raw.String), nil
	}
	if hasTools {
		data["tools"], err = s.readCollaborationTools(ctx, branch, seq)
		if err != nil {
			return nil, err
		}
	}
	if hasStates {
		data["agentsStates"], err = s.readCollaborationStates(ctx, branch, seq)
		if err != nil {
			return nil, err
		}
	}
	return json.Marshal(data)
}

func (s *Store) readCollaborationTools(ctx context.Context, branch string, seq int64) ([]any, error) {
	rows, err := s.db.QueryContext(ctx, `WITH entries AS (
 SELECT j.key,j.type,CASE WHEN j.type='object' THEN j.value ELSE '{}' END AS obj,
 CASE j.type WHEN 'true' THEN 'true' WHEN 'false' THEN 'false' WHEN 'null' THEN 'null' WHEN 'array' THEN j.value WHEN 'object' THEN j.value ELSE json_quote(j.value) END AS raw
 FROM events e,json_each(e.payload_json,'$.data.tools') j WHERE e.branch_id=? AND e.seq=?)
 SELECT key,type,CASE WHEN type='object' THEN json_remove(obj,'$.input',CASE WHEN json_type(obj,'$.output')='text' THEN '$.output' ELSE '$.input' END) ELSE raw END,
 coalesce(substr(obj -> '$.input',1,?),''),coalesce(length(CAST(obj -> '$.input' AS BLOB)),0),
 CASE WHEN json_type(obj,'$.output')='text' THEN substr(json_extract(obj,'$.output'),1,?) ELSE '' END,
 CASE WHEN json_type(obj,'$.output')='text' THEN length(CAST(json_extract(obj,'$.output') AS BLOB)) ELSE 0 END,
 coalesce(json_type(obj,'$.output')='text',0) FROM entries`, branch, seq, constants.ChatTranscriptInlineFieldBytes, constants.ChatTranscriptInlineFieldBytes)
	if err != nil {
		return nil, err
	}
	tools := []any{}
	for rows.Next() {
		var index, inputBytes, outputBytes int
		var typ, base, input, output string
		var hasOutput bool
		if err = rows.Scan(&index, &typ, &base, &input, &inputBytes, &output, &outputBytes, &hasOutput); err != nil {
			rows.Close()
			return nil, err
		}
		if typ != "object" {
			tools = append(tools, json.RawMessage(base))
			continue
		}
		var tool map[string]any
		if err = json.Unmarshal([]byte(base), &tool); err != nil {
			rows.Close()
			return nil, err
		}
		if tool == nil {
			tools = append(tools, nil)
			continue
		}
		if inputBytes > constants.ChatTranscriptInlineFieldBytes {
			tool["input"] = map[string]any{"preview": utf8Prefix(input, constants.ChatTranscriptInlineFieldBytes), "truncated": true, "contentRef": contentReference(branch, seq, "collaboration_tool_input", strconv.Itoa(index))}
		} else if inputBytes > 0 {
			tool["input"] = json.RawMessage(input)
		}
		if hasOutput {
			tool["output"] = utf8Prefix(output, constants.ChatTranscriptInlineFieldBytes)
		}
		if outputBytes > constants.ChatTranscriptInlineFieldBytes {
			tool["outputRef"] = contentReference(branch, seq, "collaboration_tool_output", strconv.Itoa(index))
			tool["outputBytes"] = outputBytes
			tool["outputTruncated"] = true
		}
		tools = append(tools, tool)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	return tools, nil
}

func (s *Store) readCollaborationStates(ctx context.Context, branch string, seq int64) (map[string]any, error) {
	rows, err := s.db.QueryContext(ctx, `WITH entries AS (
 SELECT j.key,j.type,CASE WHEN j.type='object' THEN j.value ELSE '{}' END AS obj,
 CASE j.type WHEN 'true' THEN 'true' WHEN 'false' THEN 'false' WHEN 'null' THEN 'null' WHEN 'array' THEN j.value WHEN 'object' THEN j.value ELSE json_quote(j.value) END AS raw
 FROM events e,json_each(e.payload_json,'$.data.agentsStates') j WHERE e.branch_id=? AND e.seq=?)
 SELECT key,type,CASE WHEN type='object' AND json_type(obj,'$.message')='text' THEN json_remove(obj,'$.message') ELSE raw END,
 CASE WHEN json_type(obj,'$.message')='text' THEN substr(json_extract(obj,'$.message'),1,?) ELSE '' END,
 CASE WHEN json_type(obj,'$.message')='text' THEN length(CAST(json_extract(obj,'$.message') AS BLOB)) ELSE 0 END,
 coalesce(json_type(obj,'$.message')='text',0) FROM entries`, branch, seq, constants.ChatTranscriptInlineFieldBytes)
	if err != nil {
		return nil, err
	}
	states := map[string]any{}
	for rows.Next() {
		var key, typ, base, message string
		var size int
		var hasMessage bool
		if err = rows.Scan(&key, &typ, &base, &message, &size, &hasMessage); err != nil {
			rows.Close()
			return nil, err
		}
		if typ != "object" {
			states[key] = json.RawMessage(base)
			continue
		}
		var state map[string]any
		if err = json.Unmarshal([]byte(base), &state); err != nil {
			rows.Close()
			return nil, err
		}
		if state == nil {
			states[key] = nil
			continue
		}
		if hasMessage {
			state["message"] = utf8Prefix(message, constants.ChatTranscriptInlineFieldBytes)
		}
		if size > constants.ChatTranscriptInlineFieldBytes {
			state["messageRef"] = contentReference(branch, seq, "collaboration_agent_message", key)
			state["messageBytes"] = size
			state["messageTruncated"] = true
		}
		states[key] = state
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	return states, nil
}
