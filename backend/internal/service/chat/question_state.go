package chat

import "slices"

// QuestionState distinguishes questions answered inside a live provider run
// from legacy questions whose answer starts a new run.
type QuestionState struct {
	TurnID              string   `json:"turnId,omitempty"`
	PendingInteractions []string `json:"pendingInteractions,omitempty"`
	LegacyPending       bool     `json:"legacyPending,omitempty"`
}

func IsQuestionRequest(event Event) bool {
	return (event.Type == "tool_use_start" && event.Name == "AskUserQuestion") ||
		(event.Type == "interaction_request" && event.Status == "user_input")
}

// AwaitingAnswer hides native requests after their provider process has ended.
// Legacy questions remain actionable through a fresh user prompt.
func (state QuestionState) AwaitingAnswer(running bool) bool {
	return state.LegacyPending || (running && len(state.PendingInteractions) > 0)
}

func (state QuestionState) AnsweredBy(event Event) bool {
	return event.Type == "interaction_resolved" && event.Status == "answered" &&
		state.matchesTurn(event) && slices.Contains(state.PendingInteractions, interactionID(event))
}

// Apply returns a new state without mutating slices held by cached metadata.
func (state QuestionState) Apply(event Event) (QuestionState, bool) {
	if event.Type == "user" {
		return QuestionState{}, state.TurnID != "" || state.AwaitingAnswer(true)
	}
	if IsQuestionRequest(event) {
		changed := false
		if event.TurnID != "" && event.TurnID != state.TurnID {
			state = QuestionState{TurnID: event.TurnID}
			changed = true
		}
		if event.Type == "tool_use_start" {
			changed = changed || !state.LegacyPending
			state.LegacyPending = true
			return state, changed
		}
		id := interactionID(event)
		if id == "" || slices.Contains(state.PendingInteractions, id) {
			return state, changed
		}
		state.PendingInteractions = append(slices.Clone(state.PendingInteractions), id)
		return state, true
	}
	if !state.matchesTurn(event) {
		return state, false
	}
	switch event.Type {
	case "interaction_resolved":
		id := interactionID(event)
		if !slices.Contains(state.PendingInteractions, id) {
			return state, false
		}
		state.PendingInteractions = slices.DeleteFunc(slices.Clone(state.PendingInteractions), func(pending string) bool {
			return pending == id
		})
		return state, true
	case "complete":
		changed := len(state.PendingInteractions) > 0
		state.PendingInteractions = nil
		return state, changed
	case "error":
		if event.Status == "retrying" {
			return state, false
		}
		return QuestionState{}, state.TurnID != "" || state.AwaitingAnswer(true)
	case "turn_status":
		switch event.Status {
		case "interrupted", "cancelled", "failed":
			return QuestionState{}, state.TurnID != "" || state.AwaitingAnswer(true)
		}
	}
	return state, false
}

func (state QuestionState) matchesTurn(event Event) bool {
	return event.TurnID == "" || state.TurnID == "" || event.TurnID == state.TurnID
}

func interactionID(event Event) string {
	if event.InteractionID != "" {
		return event.InteractionID
	}
	return event.ID
}
