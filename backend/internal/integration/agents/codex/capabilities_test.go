package codex

import (
	"bytes"
	"encoding/json"
	"io"
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
)

func TestReadAppServerCapabilitiesPaginatesModelCatalog(t *testing.T) {
	responses := bytes.NewBufferString(
		`{"id":1,"result":{}}` + "\n" +
			`{"id":2,"result":{"data":[{"id":"first","model":"first","displayName":"First"}],"nextCursor":"page-2"}}` + "\n" +
			`{"id":3,"result":{"data":[{"name":"Default","mode":"default"},{"name":"Plan","mode":"plan","model":"gpt-plan","reasoning_effort":"high","futureField":true}]}}` + "\n" +
			`{"id":4,"result":{"data":[{"id":"second","model":"second","displayName":"Second"}]}}` + "\n",
	)
	var requests bytes.Buffer
	models, modes, err := readAppServerCapabilities(&requests, responses)
	if err != nil {
		t.Fatal(err)
	}
	if len(models.Data) != 2 || models.Data[0].ID != "first" || models.Data[1].ID != "second" {
		t.Fatalf("models = %+v", models.Data)
	}
	if len(modes.Data) != 2 || modes.Data[1].Mode != string(agent.RunModePlan) || modes.Data[1].Model != "gpt-plan" {
		t.Fatalf("modes = %+v", modes.Data)
	}

	decoder := json.NewDecoder(&requests)
	var sent []struct {
		ID     int            `json:"id"`
		Method string         `json:"method"`
		Params map[string]any `json:"params"`
	}
	for {
		var request struct {
			ID     int            `json:"id"`
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		if err := decoder.Decode(&request); err != nil {
			if err == io.EOF {
				break
			}
			t.Fatal(err)
		}
		sent = append(sent, request)
	}
	if len(sent) != 5 || sent[1].Method != "initialized" || sent[2].Method != "model/list" || sent[4].Method != "model/list" {
		t.Fatalf("requests = %+v", sent)
	}
	if _, exists := sent[2].Params["cursor"]; exists {
		t.Fatalf("first model request has cursor: %+v", sent[2])
	}
	if sent[4].ID != 4 || sent[4].Params["cursor"] != "page-2" {
		t.Fatalf("second model request = %+v", sent[4])
	}
}

func TestBuildCapabilitiesPreservesPerModelControls(t *testing.T) {
	var models modelListResponse
	models.Data = append(models.Data, modelListItem{
		ID: "gpt-next", Model: "gpt-next", DisplayName: "GPT Next", IsDefault: true,
		DefaultReasoningEffort: "medium",
		SupportedReasoningEfforts: []reasoningEffortItem{{
			ReasoningEffort: "medium", Description: "balanced",
		}},
		ServiceTiers:    []serviceTierItem{{ID: "priority", Name: "Fast", Description: "faster"}},
		InputModalities: []string{"text", "image"}, SupportsPersonality: true,
		MultiAgentVersion: "v2", Upgrade: "gpt-next-2", Raw: json.RawMessage(`{"futureField":true}`),
	})
	modes := collaborationModeListResponse{}
	modes.Data = append(modes.Data,
		collaborationModeItem{Name: "Default", Mode: string(agent.RunModeDefault)},
		collaborationModeItem{
			Name: "Plan", Mode: string(agent.RunModePlan), Model: "gpt-next",
			ReasoningEffort: "medium", Raw: json.RawMessage(`{"futureField":true}`),
		},
	)

	caps := buildCapabilities(models, modes)
	if len(caps.Models) != 2 || caps.Models[0].ID != "" || caps.Models[1].ID != "gpt-next" {
		t.Fatalf("models = %+v", caps.Models)
	}
	if got := caps.Models[1].ReasoningEfforts; len(got) != 2 || got[1].Value != "medium" {
		t.Fatalf("reasoning efforts = %+v", got)
	}
	if got := caps.Models[1].ServiceTiers; len(got) != 2 || got[1].Value != "priority" || got[1].Label != "Fast" {
		t.Fatalf("service tiers = %+v", got)
	}
	if got := caps.Models[1]; !got.SupportsPersonality || got.MultiAgentVersion != "v2" || len(got.InputModalities) != 2 || !json.Valid(got.Raw) {
		t.Fatalf("model metadata = %+v", got)
	}
	if len(caps.Modes) != 2 || caps.Modes[0].Value != string(agent.RunModeDefault) || caps.Modes[1].Value != string(agent.RunModePlan) {
		t.Fatalf("modes = %+v", caps.Modes)
	}
	if caps.Modes[1].Model != "gpt-next" || caps.Modes[1].ReasoningEffort != "medium" || !json.Valid(caps.Modes[1].Raw) {
		t.Fatalf("plan mode metadata = %+v", caps.Modes[1])
	}
}
