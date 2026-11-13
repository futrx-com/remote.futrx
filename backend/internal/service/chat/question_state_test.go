package chat

import (
	"reflect"
	"testing"
)

func TestQuestionStateResolvesOnlyMatchingQuestions(t *testing.T) {
	var state QuestionState
	for _, id := range []string{"first", "second"} {
		state, _ = state.Apply(Event{Type: "interaction_request", Status: "user_input", TurnID: "turn", ID: id})
	}
	before := state
	duplicate, changed := state.Apply(Event{Type: "interaction_request", Status: "user_input", TurnID: "turn", ID: "first"})
	if changed || !reflect.DeepEqual(duplicate, state) {
		t.Fatal("duplicate request changed pending questions")
	}
	for _, event := range []Event{
		{Type: "interaction_resolved", TurnID: "old-turn", ID: "first", Status: "answered"},
		{Type: "interaction_resolved", TurnID: "turn", ID: "approval", Status: "approved"},
		{Type: "complete", TurnID: "old-turn"},
		{Type: "assistant_text", Text: "Working?"},
	} {
		next, changed := state.Apply(event)
		if changed || !reflect.DeepEqual(next, state) || state.AnsweredBy(event) {
			t.Fatalf("unrelated event changed question state: %+v", event)
		}
	}
	answer := Event{Type: "interaction_resolved", TurnID: "turn", InteractionID: "first", Status: "answered"}
	if !state.AnsweredBy(answer) {
		t.Fatal("matching answer was not recognized")
	}
	state, _ = state.Apply(answer)
	if !state.AwaitingAnswer(true) || !reflect.DeepEqual(state.PendingInteractions, []string{"second"}) {
		t.Fatalf("answer cleared another pending question: %+v", state)
	}
	if !reflect.DeepEqual(before.PendingInteractions, []string{"first", "second"}) {
		t.Fatal("answer mutated a cached metadata snapshot")
	}
	state, _ = state.Apply(Event{Type: "interaction_resolved", TurnID: "turn", ID: "second", Status: "server_cancelled"})
	if state.AwaitingAnswer(true) {
		t.Fatal("resolved questions still show waiting")
	}
}

func TestLegacyQuestionRemainsWaitingAfterCompletion(t *testing.T) {
	state, _ := (QuestionState{}).Apply(Event{Type: "tool_use_start", Name: "AskUserQuestion", TurnID: "turn", ID: "tool"})
	state, _ = state.Apply(Event{Type: "complete", TurnID: "turn"})
	if !state.AwaitingAnswer(false) {
		t.Fatal("legacy question must remain actionable when its run ends")
	}
	state, _ = state.Apply(Event{Type: "user", TurnID: "new-turn", Text: "the first option"})
	if state.AwaitingAnswer(true) {
		t.Fatal("a new prompt did not clear the legacy question")
	}
}

func TestNativeQuestionsRequireALiveRun(t *testing.T) {
	state, _ := (QuestionState{}).Apply(Event{Type: "interaction_request", Status: "user_input", TurnID: "turn", ID: "question"})
	if !state.AwaitingAnswer(true) || state.AwaitingAnswer(false) {
		t.Fatal("native question status must follow provider run availability")
	}
	state, _ = state.Apply(Event{Type: "complete", TurnID: "turn"})
	if state.AwaitingAnswer(true) {
		t.Fatal("completed run retained native questions")
	}
}

func TestQuestionStateClearsOnFailureOrInterruption(t *testing.T) {
	for _, event := range []Event{
		{Type: "error"},
		{Type: "turn_status", Status: "interrupted"},
		{Type: "turn_status", Status: "cancelled"},
		{Type: "turn_status", Status: "failed"},
		{Type: "user"},
	} {
		state := QuestionState{TurnID: "turn", PendingInteractions: []string{"question"}, LegacyPending: true}
		state, _ = state.Apply(event)
		if state.AwaitingAnswer(true) {
			t.Fatalf("question survived termination: %+v", event)
		}
	}
	state := QuestionState{TurnID: "turn", PendingInteractions: []string{"question"}}
	next, changed := state.Apply(Event{Type: "error", Status: "retrying"})
	if changed || !next.AwaitingAnswer(true) {
		t.Fatal("retryable provider error cleared an actionable question")
	}
}

func TestFreshQuestionTurnReplacesStaleRequests(t *testing.T) {
	state := QuestionState{TurnID: "old", PendingInteractions: []string{"old-question"}, LegacyPending: true}
	state, _ = state.Apply(Event{Type: "interaction_request", Status: "user_input", TurnID: "new", ID: "new-question"})
	if state.LegacyPending || state.TurnID != "new" || !reflect.DeepEqual(state.PendingInteractions, []string{"new-question"}) {
		t.Fatalf("new turn retained stale questions: %+v", state)
	}
}

func TestQuestionRequestsExcludeOtherInteractions(t *testing.T) {
	for _, status := range []string{"approval", "permission", "elicitation", "provider_request", ""} {
		event := Event{Type: "interaction_request", Status: status, ID: "request"}
		state, changed := (QuestionState{}).Apply(event)
		if IsQuestionRequest(event) || changed || state.AwaitingAnswer(true) {
			t.Fatalf("non-question interaction became a waiting question: %s", status)
		}
	}
}
