package codexharness

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
)

func respondToRequest(t *testing.T, method, params, result string) agent.Event {
	t.Helper()
	var events []agent.Event
	handler := newAppServerRequestHandler(
		agent.RunRequest{ConversationID: "chat-1"},
		func(event agent.Event) { events = append(events, event) },
		func(any) error { return nil },
	)
	if err := handler.Handle(appServerEnvelope{ID: []byte("7"), Method: method, Params: []byte(params)}); err != nil {
		t.Fatal(err)
	}
	if err := handler.Respond(agent.InteractionResponse{ID: "7", Result: []byte(result)}); err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[1].Type != agent.EventInteractionDone {
		t.Fatalf("events = %#v", events)
	}
	return events[1]
}

func TestAppServerRecordsAnswersOnTheResolvedEvent(t *testing.T) {
	resolved := respondToRequest(t,
		"item/tool/requestUserInput",
		`{"questions":[
			{"id":"env","question":"Which environment?","options":[{"label":"staging"},{"label":"production"}]},
			{"question":"Anything else?","options":[]}
		]}`,
		`{"answers":{"env":{"answers":["staging"]},"1":{"answers":["ship it"]}}}`,
	)

	var data struct {
		Answers map[string][]string `json:"answers"`
	}
	if err := json.Unmarshal(resolved.Data, &data); err != nil {
		t.Fatalf("resolved data = %s: %v", resolved.Data, err)
	}
	if got := data.Answers["env"]; len(got) != 1 || got[0] != "staging" {
		t.Fatalf("env answers = %q, want [staging]", got)
	}
	// A question without an id is keyed by its index, as the browser keys it.
	if got := data.Answers["1"]; len(got) != 1 || got[0] != "ship it" {
		t.Fatalf("index-keyed answers = %q, want [ship it]", got)
	}
}

func TestAppServerRecordsOnlyTheNonSecretAnswers(t *testing.T) {
	resolved := respondToRequest(t,
		"item/tool/requestUserInput",
		`{"questions":[
			{"id":"token","question":"Token?","isSecret":true,"options":[]},
			{"id":"region","question":"Region?","options":[]}
		]}`,
		`{"answers":{"token":{"answers":["super-secret-value"]},"region":{"answers":["eu-west-1"]}}}`,
	)

	if strings.Contains(string(resolved.Data), "super-secret-value") || strings.Contains(string(resolved.Data), `"token"`) {
		t.Fatalf("secret answer recorded: %s", resolved.Data)
	}
	if !strings.Contains(string(resolved.Data), "eu-west-1") {
		t.Fatalf("resolved data = %s, want the non-secret answer", resolved.Data)
	}
}

func TestAppServerRecordsNothingForAnAllSecretPromptOrAnApproval(t *testing.T) {
	secret := respondToRequest(t,
		"item/tool/requestUserInput",
		`{"questions":[{"id":"token","question":"Token?","isSecret":true,"options":[]}]}`,
		`{"answers":{"token":{"answers":["super-secret-value"]}}}`,
	)
	if len(secret.Data) != 0 {
		t.Fatalf("all-secret prompt data = %s, want none", secret.Data)
	}

	approval := respondToRequest(t,
		"item/commandExecution/requestApproval",
		`{"command":"rm -rf build"}`,
		`{"decision":"accept"}`,
	)
	if len(approval.Data) != 0 || approval.Status != "approved" {
		t.Fatalf("approval = %#v, want no recorded data", approval)
	}
}
