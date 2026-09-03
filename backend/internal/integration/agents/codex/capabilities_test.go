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
			`{"id":3,"result":{"data":[{"name":"Plan","mode":"plan"}]}}` + "\n" +
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
	if len(modes.Data) != 1 || modes.Data[0].Mode != string(agent.RunModePlan) {
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
	if len(sent) != 4 || sent[1].Method != "model/list" || sent[3].Method != "model/list" {
		t.Fatalf("requests = %+v", sent)
	}
	if _, exists := sent[1].Params["cursor"]; exists {
		t.Fatalf("first model request has cursor: %+v", sent[1])
	}
	if sent[3].ID != 4 || sent[3].Params["cursor"] != "page-2" {
		t.Fatalf("second model request = %+v", sent[3])
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
		ServiceTiers: []serviceTierItem{{ID: "priority", Name: "Fast", Description: "faster"}},
	})
	modes := collaborationModeListResponse{}
	modes.Data = append(modes.Data, collaborationModeItem{Name: "Plan", Mode: string(agent.RunModePlan)})

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
	if len(caps.Modes) != 2 || caps.Modes[0].Value != string(agent.RunModeDefault) || caps.Modes[1].Value != string(agent.RunModePlan) {
		t.Fatalf("modes = %+v", caps.Modes)
	}
}
