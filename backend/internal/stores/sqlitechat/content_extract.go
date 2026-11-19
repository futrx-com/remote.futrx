package sqlitechat

import (
	"encoding/json"
	"fmt"
	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
	"strconv"
)

func extractTranscriptContent(event servicechat.Event, fieldKind, fieldKey string) (string, error) {
	switch fieldKind {
	case "tool_output":
		return event.Output, nil
	case "tool_input":
		return string(event.Input), nil
	}
	var data map[string]any
	if json.Unmarshal(event.Data, &data) != nil {
		return "", fmt.Errorf("%w: invalid collaboration data", errInvalidProjection)
	}
	switch fieldKind {
	case "collaboration_tool_output", "collaboration_tool_input":
		index, err := strconv.Atoi(fieldKey)
		tools, ok := data["tools"].([]any)
		if err != nil || !ok || index < 0 || index >= len(tools) {
			return "", fmt.Errorf("%w: collaboration tool reference", errInvalidProjection)
		}
		tool, ok := tools[index].(map[string]any)
		if !ok {
			return "", fmt.Errorf("%w: collaboration tool payload", errInvalidProjection)
		}
		if fieldKind == "collaboration_tool_output" {
			output, ok := tool["output"].(string)
			if !ok {
				return "", fmt.Errorf("%w: collaboration tool output", errInvalidProjection)
			}
			return output, nil
		}
		input, ok := tool["input"]
		if !ok {
			return "", fmt.Errorf("%w: collaboration tool input", errInvalidProjection)
		}
		encoded, err := json.Marshal(input)
		return string(encoded), err
	case "collaboration_agent_message":
		states, ok := data["agentsStates"].(map[string]any)
		if !ok {
			return "", fmt.Errorf("%w: collaboration states", errInvalidProjection)
		}
		state, ok := states[fieldKey].(map[string]any)
		if !ok {
			return "", fmt.Errorf("%w: collaboration state", errInvalidProjection)
		}
		message, ok := state["message"].(string)
		if !ok {
			return "", fmt.Errorf("%w: collaboration message", errInvalidProjection)
		}
		return message, nil
	default:
		return "", servicechat.ErrTranscriptContentNotFound
	}
}
