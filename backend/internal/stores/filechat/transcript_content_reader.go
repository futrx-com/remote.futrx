package filechat

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"unicode/utf8"

	configconstants "github.com/futrx-com/remote.futrx.com/internal/config/constants"
	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
)

func (s *Store) ReadTranscriptContent(
	ctx context.Context,
	id servicechat.ID,
	contentID string,
	afterBytes int64,
	limitBytes int,
) (servicechat.TranscriptContentPage, error) {
	if !servicechat.ValidID(id) {
		return servicechat.TranscriptContentPage{}, servicechat.ErrInvalidID
	}
	if contentID == "" || afterBytes < 0 {
		return servicechat.TranscriptContentPage{}, servicechat.ErrTranscriptContentNotFound
	}
	if err := s.index.availabilityError(); err != nil {
		return servicechat.TranscriptContentPage{}, fmt.Errorf(
			"%w: %v", servicechat.ErrTranscriptProjectionUnavailable, err,
		)
	}
	if limitBytes <= 0 {
		limitBytes = configconstants.DefaultChatTranscriptContentPageBytes
	}
	if limitBytes < utf8.UTFMax {
		limitBytes = utf8.UTFMax
	}
	if limitBytes > configconstants.MaxChatTranscriptContentPageBytes {
		limitBytes = configconstants.MaxChatTranscriptContentPageBytes
	}

	ref, err := s.index.readTranscriptContentRef(ctx, id, contentID)
	if err != nil {
		return servicechat.TranscriptContentPage{}, err
	}
	event, err := readProjectedSourceEvent(ctx, s.eventsPath(id), ref.sourceOffset, ref.sourceLength)
	if err != nil {
		return servicechat.TranscriptContentPage{}, err
	}
	content, err := extractTranscriptContent(event, ref.fieldKind, ref.fieldKey)
	if err != nil {
		return servicechat.TranscriptContentPage{}, err
	}
	if int64(len(content)) != ref.contentBytes {
		return servicechat.TranscriptContentPage{}, fmt.Errorf(
			"%w: projected content changed", errInvalidChatEventIndex,
		)
	}
	return transcriptContentChunk(contentID, content, afterBytes, limitBytes)
}

func (index *chatEventIndex) readTranscriptContentRef(
	ctx context.Context,
	id servicechat.ID,
	contentID string,
) (transcriptContentRef, error) {
	ref := transcriptContentRef{id: contentID}
	err := index.db.QueryRowContext(ctx, `
		SELECT field_kind, field_key, source_offset, source_length, content_bytes
		FROM chat_transcript_content_refs
		WHERE chat_id = ? AND content_id = ?`, id, contentID,
	).Scan(&ref.fieldKind, &ref.fieldKey, &ref.sourceOffset, &ref.sourceLength, &ref.contentBytes)
	if errors.Is(err, sql.ErrNoRows) {
		return transcriptContentRef{}, servicechat.ErrTranscriptContentNotFound
	}
	return ref, err
}

func transcriptContentChunk(
	contentID string,
	content string,
	afterBytes int64,
	limitBytes int,
) (servicechat.TranscriptContentPage, error) {
	if afterBytes > int64(len(content)) {
		return servicechat.TranscriptContentPage{}, servicechat.ErrTranscriptContentNotFound
	}
	start := int(afterBytes)
	for start < len(content) && !utf8.RuneStart(content[start]) {
		start++
	}
	end := start + limitBytes
	if end > len(content) {
		end = len(content)
	}
	for end > start && end < len(content) && !utf8.RuneStart(content[end]) {
		end--
	}
	complete := end == len(content)
	var nextAfter int64
	if !complete {
		nextAfter = int64(end)
	}
	return servicechat.TranscriptContentPage{
		ContentID:  contentID,
		Content:    content[start:end],
		NextAfter:  nextAfter,
		TotalBytes: int64(len(content)),
		Complete:   complete,
	}, nil
}

func readProjectedSourceEvent(
	ctx context.Context,
	eventsPath string,
	offset int64,
	length int64,
) (servicechat.Event, error) {
	if offset < 0 || length <= 0 || length > maxEventRecordBytes+2 {
		return servicechat.Event{}, fmt.Errorf("%w: content source range", errInvalidChatEventIndex)
	}
	file, err := os.Open(eventsPath)
	if err != nil {
		return servicechat.Event{}, err
	}
	defer file.Close()
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return servicechat.Event{}, err
	}
	raw := make([]byte, int(length))
	if _, err := io.ReadFull(file, raw); err != nil {
		return servicechat.Event{}, indexedReadError(err)
	}
	if err := ctx.Err(); err != nil {
		return servicechat.Event{}, err
	}
	raw = bytes.TrimSuffix(raw, []byte{'\n'})
	raw = bytes.TrimSuffix(raw, []byte{'\r'})
	event, err := decodeStoredEvent(raw, 0)
	if err != nil {
		return servicechat.Event{}, fmt.Errorf("%w: decode content source: %v", errInvalidChatEventIndex, err)
	}
	return event, nil
}

func extractTranscriptContent(event servicechat.Event, fieldKind, fieldKey string) (string, error) {
	switch fieldKind {
	case "tool_output":
		return event.Output, nil
	case "tool_input":
		return string(event.Input), nil
	}
	var data map[string]any
	if json.Unmarshal(event.Data, &data) != nil {
		return "", fmt.Errorf("%w: invalid collaboration data", errInvalidChatEventIndex)
	}
	switch fieldKind {
	case "collaboration_tool_output", "collaboration_tool_input":
		index, err := strconv.Atoi(fieldKey)
		tools, ok := data["tools"].([]any)
		if err != nil || !ok || index < 0 || index >= len(tools) {
			return "", fmt.Errorf("%w: collaboration tool reference", errInvalidChatEventIndex)
		}
		tool, ok := tools[index].(map[string]any)
		if !ok {
			return "", fmt.Errorf("%w: collaboration tool payload", errInvalidChatEventIndex)
		}
		if fieldKind == "collaboration_tool_output" {
			output, ok := tool["output"].(string)
			if !ok {
				return "", fmt.Errorf("%w: collaboration tool output", errInvalidChatEventIndex)
			}
			return output, nil
		}
		input, ok := tool["input"]
		if !ok {
			return "", fmt.Errorf("%w: collaboration tool input", errInvalidChatEventIndex)
		}
		encoded, err := json.Marshal(input)
		return string(encoded), err
	case "collaboration_agent_message":
		states, ok := data["agentsStates"].(map[string]any)
		if !ok {
			return "", fmt.Errorf("%w: collaboration states", errInvalidChatEventIndex)
		}
		state, ok := states[fieldKey].(map[string]any)
		if !ok {
			return "", fmt.Errorf("%w: collaboration state", errInvalidChatEventIndex)
		}
		message, ok := state["message"].(string)
		if !ok {
			return "", fmt.Errorf("%w: collaboration message", errInvalidChatEventIndex)
		}
		return message, nil
	default:
		return "", servicechat.ErrTranscriptContentNotFound
	}
}
