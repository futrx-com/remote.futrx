package filechat

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"strings"
	"unicode/utf8"

	configconstants "github.com/futrx-com/remote.futrx.com/internal/config/constants"
	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
)

type transcriptContentRef struct {
	id           string
	fieldKind    string
	fieldKey     string
	sourceOffset int64
	sourceLength int64
	contentBytes int64
}

type transcriptItemMode uint8

const (
	transcriptItemSingle transcriptItemMode = iota
	transcriptItemReplace
	transcriptItemAppend
)

func transcriptItemIdentity(event servicechat.Event, ordinal int64) (string, transcriptItemMode, bool) {
	switch event.Type {
	case "provider_event", "usage_update", "session", "sync", "system", "permission_request":
		return "", transcriptItemSingle, false
	case "collaboration":
		if event.Name == "wait" {
			return "", transcriptItemSingle, false
		}
		if event.ID != "" {
			return "collaboration:" + event.ID, transcriptItemReplace, true
		}
	case "turn_status":
		return "turn-status", transcriptItemReplace, true
	case "tool_use_start", "tool_use_end":
		if event.ID != "" {
			return "tool:" + event.ID, transcriptItemAppend, true
		}
	case "interaction_request", "interaction_resolved":
		if event.ID != "" {
			return "interaction:" + event.ID, transcriptItemAppend, true
		}
	}
	return "event:" + strconv.FormatInt(ordinal, 10), transcriptItemSingle, true
}

func compactTranscriptEvent(
	chatID servicechat.ID,
	event servicechat.Event,
	itemKey string,
	sourceOffset int64,
	sourceLength int64,
	turnOrdinal int64,
) (servicechat.Event, []transcriptContentRef) {
	event.Native = nil
	makeRef := func(fieldKind, fieldKey string, contentBytes int) transcriptContentRef {
		sum := sha256.Sum256([]byte(strings.Join([]string{
			string(chatID),
			strconv.FormatInt(turnOrdinal, 10),
			itemKey,
			fieldKind,
			fieldKey,
		}, "\x00")))
		return transcriptContentRef{
			id:           hex.EncodeToString(sum[:16]),
			fieldKind:    fieldKind,
			fieldKey:     fieldKey,
			sourceOffset: sourceOffset,
			sourceLength: sourceLength,
			contentBytes: int64(contentBytes),
		}
	}

	var refs []transcriptContentRef
	switch event.Type {
	case "tool_use_start":
		if len(event.Input) > configconstants.ChatTranscriptInlineFieldBytes {
			ref := makeRef("tool_input", "", len(event.Input))
			refs = append(refs, ref)
			preview, _ := json.Marshal(map[string]any{
				"preview":    utf8Prefix(string(event.Input), configconstants.ChatTranscriptInlineFieldBytes),
				"truncated":  true,
				"contentRef": ref.id,
			})
			event.Input = preview
		}
	case "tool_use_end":
		if len(event.Output) > configconstants.ChatTranscriptInlineFieldBytes {
			fullBytes := len(event.Output)
			ref := makeRef("tool_output", "", fullBytes)
			refs = append(refs, ref)
			event.Output = utf8Prefix(event.Output, configconstants.ChatTranscriptInlineFieldBytes)
			event.OutputRef = ref.id
			event.OutputBytes = int64(fullBytes)
			event.OutputTruncated = true
		}
	case "collaboration":
		event.Data, refs = compactCollaborationData(event.Data, makeRef)
	}
	return event, refs
}

func compactCollaborationData(
	raw json.RawMessage,
	makeRef func(string, string, int) transcriptContentRef,
) (json.RawMessage, []transcriptContentRef) {
	var data map[string]any
	if len(raw) == 0 || json.Unmarshal(raw, &data) != nil {
		return raw, nil
	}
	var refs []transcriptContentRef
	if tools, ok := data["tools"].([]any); ok {
		for index, value := range tools {
			tool, ok := value.(map[string]any)
			if !ok {
				continue
			}
			key := strconv.Itoa(index)
			if output, ok := tool["output"].(string); ok && len(output) > configconstants.ChatTranscriptInlineFieldBytes {
				ref := makeRef("collaboration_tool_output", key, len(output))
				refs = append(refs, ref)
				tool["output"] = utf8Prefix(output, configconstants.ChatTranscriptInlineFieldBytes)
				tool["outputRef"] = ref.id
				tool["outputBytes"] = len(output)
				tool["outputTruncated"] = true
			}
			if input, exists := tool["input"]; exists {
				encoded, err := json.Marshal(input)
				if err == nil && len(encoded) > configconstants.ChatTranscriptInlineFieldBytes {
					ref := makeRef("collaboration_tool_input", key, len(encoded))
					refs = append(refs, ref)
					tool["input"] = map[string]any{
						"preview":    utf8Prefix(string(encoded), configconstants.ChatTranscriptInlineFieldBytes),
						"truncated":  true,
						"contentRef": ref.id,
					}
				}
			}
		}
	}
	if states, ok := data["agentsStates"].(map[string]any); ok {
		for threadID, value := range states {
			state, ok := value.(map[string]any)
			if !ok {
				continue
			}
			if message, ok := state["message"].(string); ok && len(message) > configconstants.ChatTranscriptInlineFieldBytes {
				ref := makeRef("collaboration_agent_message", threadID, len(message))
				refs = append(refs, ref)
				state["message"] = utf8Prefix(message, configconstants.ChatTranscriptInlineFieldBytes)
				state["messageRef"] = ref.id
				state["messageBytes"] = len(message)
				state["messageTruncated"] = true
			}
		}
	}
	encoded, err := json.Marshal(data)
	if err != nil {
		return raw, nil
	}
	return encoded, refs
}

func utf8Prefix(value string, limit int) string {
	if limit <= 0 || len(value) <= limit {
		return value
	}
	end := limit
	for end > 0 && !utf8.RuneStart(value[end]) {
		end--
	}
	return value[:end]
}
