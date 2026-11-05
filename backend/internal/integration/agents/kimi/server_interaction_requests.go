package kimi

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
)

type nativeQuestion struct {
	ID       string `json:"id"`
	Question string `json:"question"`
	Header   string `json:"header"`
	Body     string `json:"body"`
	Options  []struct {
		ID          string `json:"id"`
		Label       string `json:"label"`
		Description string `json:"description"`
	} `json:"options"`
	MultiSelect bool `json:"multi_select"`
	AllowOther  bool `json:"allow_other"`
}
type pendingInteraction struct {
	kind, id  string
	questions []nativeQuestion
}

// interactionRequests owns pending identity and request/resolution projections.
// It never publishes private resolution payloads or transports responses.
type interactionRequests struct{ pending map[string]pendingInteraction }

func newInteractionRequests() interactionRequests {
	return interactionRequests{pending: map[string]pendingInteraction{}}
}
func (i *interactionRequests) active() bool { return len(i.pending) > 0 }
func (i *interactionRequests) get(id string) (pendingInteraction, bool) {
	p, ok := i.pending[id]
	return p, ok
}
func (i *interactionRequests) resolve(id string) bool {
	_, pending := i.pending[id]
	delete(i.pending, id)
	return pending
}

func (i *interactionRequests) event(kind string, raw json.RawMessage) (*agent.Event, error) {
	var p struct {
		AgentID    string           `json:"agentId"`
		ApprovalID string           `json:"approval_id"`
		QuestionID string           `json:"question_id"`
		ToolName   string           `json:"tool_name"`
		Action     string           `json:"action"`
		Display    json.RawMessage  `json:"tool_input_display"`
		Questions  []nativeQuestion `json:"questions"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, err
	}
	family, _, _ := strings.Cut(kind, ".")
	id := p.ApprovalID
	if family == "question" {
		id = p.QuestionID
	}
	if id == "" {
		return nil, fmt.Errorf("Kimi %s has no request ID", kind)
	}
	key := family + ":" + id
	ev := agent.Event{Type: agent.EventInteractionDone, InteractionID: key, ToolName: "kimi/" + family, Status: "resolved"}
	// Resolution payloads can contain private answers; never persist them.
	if !strings.HasSuffix(kind, ".requested") {
		delete(i.pending, key)
		return &ev, nil
	}
	if _, exists := i.pending[key]; exists {
		return nil, nil
	}
	i.pending[key] = pendingInteraction{kind: family, id: id, questions: p.Questions}
	ev.Type = agent.EventInteractionRequest
	input := map[string]any{"agentId": p.AgentID, "tool": p.ToolName, "reason": p.Action, "display": p.Display}
	if family == "question" {
		ev.Status = "user_input"
		questions := make([]any, 0, len(p.Questions))
		for _, q := range p.Questions {
			questions = append(questions, map[string]any{"id": q.ID, "question": q.Question, "header": q.Header, "body": q.Body, "options": q.Options, "multiSelect": q.MultiSelect, "isOther": q.AllowOther})
		}
		input = map[string]any{"questions": questions, "allowDismiss": true}
	} else {
		ev.Status = "approval"
		input["allowFeedback"] = true
	}
	ev.Input, _ = json.Marshal(input)
	return &ev, nil
}
