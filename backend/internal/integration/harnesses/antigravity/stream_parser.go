package antigravity

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/harness"
)

// streamParser uses the documented headless NDJSON protocol. In particular,
// user_input DONE is acceptance evidence; init only identifies the session.
// https://antigravity.google/docs/cli/headless/
type streamParser struct {
	req          harness.RunRequest
	sessionID    string
	accepted     bool
	sawText      bool
	toolStarted  map[int]bool
	toolFinished map[int]bool
}

func newStreamParser(req harness.RunRequest) *streamParser {
	return &streamParser{req: req, sessionID: req.ResumeID, toolStarted: map[int]bool{}, toolFinished: map[int]bool{}}
}

type streamOutput struct {
	Event          string `json:"event"`
	ConversationID string `json:"conversation_id"`
	Step           struct {
		ConversationID string `json:"conversation_id"`
		Index          int    `json:"step_index"`
		State          string `json:"state"`
		Type           string `json:"step_type"`
		Text           string `json:"text_delta"`
		ToolName       string `json:"tool_name"`
		ToolInfo       struct {
			Name       string          `json:"name"`
			Parameters json.RawMessage `json:"parameters"`
			Output     json.RawMessage `json:"output"`
			Error      json.RawMessage `json:"error"`
		} `json:"tool_info"`
	} `json:"step_update"`
	Result struct {
		ConversationID string  `json:"conversation_id"`
		Status         string  `json:"status"`
		Response       string  `json:"response"`
		Error          string  `json:"error"`
		Duration       float64 `json:"duration_seconds"`
	} `json:"result"`
}

func (p *streamParser) ParseLine(line []byte) ([]harness.Event, error) {
	var raw streamOutput
	if err := json.Unmarshal(line, &raw); err != nil {
		return nil, err
	}
	native := raw.ConversationID
	if raw.Step.ConversationID != "" {
		native = raw.Step.ConversationID
	}
	if raw.Result.ConversationID != "" {
		native = raw.Result.ConversationID
	}
	var events []harness.Event
	event := func(kind harness.EventType) harness.Event {
		return harness.Event{T: time.Now().UnixMilli(), Type: kind, Provider: harness.ProviderAntigravity, ConversationID: p.req.ConversationID,
			Raw: append(json.RawMessage(nil), line...), Native: &harness.NativeEnvelope{SchemaVersion: harness.NativeEnvelopeSchemaVersion, Method: raw.Event, ThreadID: p.sessionID, Payload: append(json.RawMessage(nil), line...)}}
	}
	if native != "" && native != p.sessionID {
		p.sessionID = native
		e := event(harness.EventSessionUpdated)
		e.SessionID = native
		events = append(events, e)
	}
	switch raw.Event {
	case "init":
		e := event(harness.EventSystem)
		e.Subtype = "init"
		events = append(events, e)
	case "step_update":
		step := raw.Step
		itemID := p.sessionID + ":" + strconv.Itoa(step.Index)
		switch step.Type {
		case "user_input":
			if step.State == "DONE" && !p.accepted && p.sessionID != "" {
				p.accepted = true
				e := event(harness.EventInputAccepted)
				e.SessionID = p.sessionID
				e.Status = "antigravity_user_input_done"
				e.Native.ItemID = itemID
				events = append(events, e)
			}
		case "agent_response":
			if step.Text != "" {
				p.sawText = true
				e := event(harness.EventAssistantTextDelta)
				e.Text = step.Text
				e.MessageID = itemID
				e.ItemKind = harness.ItemMessage
				events = append(events, e)
			}
		case "tool":
			name := step.ToolName
			if step.ToolInfo.Name != "" {
				name = step.ToolInfo.Name
			}
			if !p.toolStarted[step.Index] {
				p.toolStarted[step.Index] = true
				e := event(harness.EventToolStarted)
				e.ItemID = itemID
				e.ToolName = name
				e.Input = step.ToolInfo.Parameters
				events = append(events, e)
			}
			if step.State == "DONE" && !p.toolFinished[step.Index] {
				p.toolFinished[step.Index] = true
				e := event(harness.EventToolCompleted)
				e.ItemID = itemID
				e.ToolName = name
				e.Output = streamToolOutput(step.ToolInfo.Output)
				e.IsError = len(step.ToolInfo.Error) > 0 && string(step.ToolInfo.Error) != "null"
				if e.IsError && e.Output == "" {
					e.Output = string(step.ToolInfo.Error)
				}
				events = append(events, e)
			}
		}
	case "result":
		if !p.sawText && raw.Result.Response != "" {
			e := event(harness.EventAssistantTextDelta)
			e.Text = raw.Result.Response
			e.ItemKind = harness.ItemMessage
			events = append(events, e)
		}
		switch raw.Result.Status {
		case "SUCCESS":
			e := event(harness.EventRunCompleted)
			// Native result counters can be cumulative across a session. Retain them
			// in the native payload; do not mislabel them as per-run token usage.
			e.Usage = harness.Usage{Model: p.req.Model}.Raw()
			events = append(events, e)
		case "CANCELLED", "CANCELED", "INTERRUPTED":
			e := event(harness.EventRunInterrupted)
			e.Status = "interrupted"
			events = append(events, e)
		default:
			e := event(harness.EventRunFailed)
			e.Message = raw.Result.Error
			if e.Message == "" {
				e.Message = fmt.Sprintf("Antigravity returned status %q", raw.Result.Status)
			}
			events = append(events, e)
		}
	}
	// Unknown events/step types still retain their opaque payload for storage.
	if len(events) == 0 {
		events = append(events, event(harness.EventProviderNative))
	}
	return events, nil
}
func streamToolOutput(data json.RawMessage) string {
	var text string
	if json.Unmarshal(data, &text) == nil {
		return text
	}
	return string(data)
}
