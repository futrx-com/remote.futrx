package kimicodex

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	agentauth "github.com/futrx-com/remote.futrx.com/internal/service/agent/auth"
)

const catalogFixture = `{"data":[
 {"id":"kimi-for-coding","context_length":1048576},
 {"id":"k3","display_name":"Kimi K3","context_length":1048576,
  "think_efforts":{"support":true,"valid_efforts":["low","high","max"],"default_effort":"high"}},
 {"id":"k3-256k","context_length":262144},
 {"id":"kimi-for-coding-highspeed","context_length":262144,"supports_image_in":false},
 {"id":"k3.1-pro","context_length":262144},
 {"id":"k3-256k"}, {"id":"unrelated-image-model"},
 {"id":"k3-no-tools","supports_tool_use":false}
]}`

func TestModelDiscoveryAndKeyValidation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/models" || r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("unexpected discovery request: %s %s", r.Method, r.URL.Path)
		}
		fmt.Fprint(w, catalogFixture)
	}))
	defer server.Close()
	client := newModelCatalogClient()
	client.endpoint = server.URL + "/models"
	if err := client.ValidateAPIKey(context.Background(), "test-key"); err != nil {
		t.Fatal(err)
	}
	models, err := client.Models(context.Background(), "test-key")
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 5 || models[0].ID != "k3-256k" {
		t.Fatalf("models = %#v", models)
	}
	k3, err := resolveModel(models, "k3")
	if err != nil || k3.ContextWindow != 1048576 || k3.Label != "Kimi K3" {
		t.Fatalf("K3 metadata = %#v, %v", k3, err)
	}
	for input, want := range map[agent.ReasoningEffort]agent.ReasoningEffort{
		"": "high", "none": "high", "medium": "high", "low": "low", "max": "max",
	} {
		if got := k3.reasoningEffort(input); got != want {
			t.Fatalf("effort(%q) = %q", input, got)
		}
	}
	if model, err := resolveModel(models, ""); err != nil || model.ID != "k3-256k" {
		t.Fatalf("default = %#v, %v", model, err)
	}
	if _, err := resolveModel(models, "unavailable"); !errors.Is(err, ErrKimiCodexModelUnavailable) {
		t.Fatalf("unknown model error = %v", err)
	}
}

func TestDiscoveryRejectsErrorsWithoutLeakingCredentials(t *testing.T) {
	for _, tc := range []struct {
		name     string
		status   int
		body     string
		rejected bool
	}{
		{"unauthorized", 401, "test-key", true}, {"forbidden", 403, "test-key", true},
		{"rate limited", 429, "test-key", false}, {"invalid JSON", 200, "test-key", false},
		{"empty models", 200, `{"data":[]}`, false},
		{"too large", 200, strings.Repeat("x", maxModelCatalogResponseBytes+1), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			}))
			defer server.Close()
			client := newModelCatalogClient()
			client.endpoint = server.URL
			err := client.ValidateAPIKey(context.Background(), "test-key")
			if !errors.Is(err, ErrKimiCodexModelDiscoveryUnavailable) || strings.Contains(err.Error(), "test-key") {
				t.Fatalf("error = %v", err)
			}
			if errors.Is(err, agentauth.ErrAPIKeyRejected) != tc.rejected {
				t.Fatalf("rejection = %v", err)
			}
		})
	}
}

func TestDiscoveryNeverFollowsCredentialRedirect(t *testing.T) {
	visited := false
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { visited = true }))
	defer target.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer origin.Close()
	client := newModelCatalogClient()
	client.endpoint = origin.URL
	if err := client.ValidateAPIKey(context.Background(), "test-key"); err == nil || visited {
		t.Fatalf("redirect followed = %v; error = %v", visited, err)
	}
}

func TestContextAndEffortMetadataStayWithinModelLimits(t *testing.T) {
	models, err := parseModels([]byte(`{"data":[
	 {"id":"k3-256k","context_length":1048576},
	 {"id":"k3","think_efforts":{"support":true,"valid_efforts":["none","max","max"],"default_effort":"none"}}
	]}`))
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range models {
		if m.ContextWindow != 262144 {
			t.Fatalf("context = %d", m.ContextWindow)
		}
	}
	if !slices.Equal(models[1].Efforts, []agent.ReasoningEffort{"max"}) || models[1].DefaultEffort != "max" {
		t.Fatalf("efforts = %#v", models[1])
	}
}

func TestCapabilitiesAndRuntimeCatalogUseTheSameModelMetadata(t *testing.T) {
	models, err := parseModels([]byte(catalogFixture))
	if err != nil {
		t.Fatal(err)
	}
	p := &Provider{apiKeys: kimiCodexTestAPIKeys{key: "test-key"}, models: kimiCodexTestModels{models: models}}
	caps, err := p.Capabilities(context.Background(), agent.CapabilityRequest{})
	if err != nil || caps.Provider != agent.ProviderKimiCodex || caps.Source != agent.CapabilitySourceLive || len(caps.Models) != 6 {
		t.Fatalf("capabilities = %#v, %v", caps, err)
	}
	if !caps.Models[1].ProviderDefault || caps.Models[1].ID != "k3-256k" || caps.Models[1].DefaultReasoningEffort != "high" {
		t.Fatalf("default = %#v", caps.Models[1])
	}
	data, err := buildRuntimeModelCatalog(models)
	if err != nil {
		t.Fatal(err)
	}
	var catalog struct {
		Models []runtimeModel `json:"models"`
	}
	if err := json.Unmarshal(data, &catalog); err != nil {
		t.Fatal(err)
	}
	for i, runtime := range catalog.Models {
		if runtime.Slug != models[i].ID || runtime.ContextWindow != models[i].ContextWindow ||
			runtime.DefaultReasoningLevel != models[i].DefaultEffort || runtime.ApplyPatchToolType != "freeform" {
			t.Fatalf("runtime metadata = %#v", runtime)
		}
		if slices.Contains(runtime.InputModalities, "video") || slices.Contains(runtime.InputModalities, "image") != models[i].ImageInput {
			t.Fatalf("modalities = %v", runtime.InputModalities)
		}
	}
}

func TestMissingKeyOrFailedDiscoveryDoesNotAdvertiseModels(t *testing.T) {
	for _, p := range []*Provider{
		{}, {apiKeys: kimiCodexTestAPIKeys{key: "test-key"}, models: kimiCodexTestModels{err: ErrKimiCodexModelDiscoveryUnavailable}},
	} {
		caps, err := p.Capabilities(context.Background(), agent.CapabilityRequest{})
		if err == nil || caps.Source != agent.CapabilitySourceFallback || len(caps.Models) != 1 || caps.Models[0].ID != "" {
			t.Fatalf("fallback = %#v, %v", caps, err)
		}
	}
}

type kimiCodexTestModels struct {
	models []modelInfo
	err    error
}

func (s kimiCodexTestModels) Models(context.Context, string) ([]modelInfo, error) {
	return s.models, s.err
}
