package minimax

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	agentauth "github.com/futrx-com/remote.futrx.com/internal/service/agent/auth"
)

type planKeySource struct {
	keys     map[string]string
	accounts []agentauth.Account
}

func (s planKeySource) APIKey() (string, bool) { return s.APIKeyFor("") }
func (s planKeySource) APIKeyFor(id string) (string, bool) {
	key, ok := s.keys[id]
	return key, ok
}
func (s planKeySource) AccountsEnabled() bool { return s.accounts != nil }
func (s planKeySource) AccountsSnapshot() agentauth.AccountsSnapshot {
	return agentauth.AccountsSnapshot{Items: s.accounts}
}

func TestMiniMaxReadsEachSavedKeyAndOnlyTheCodingBucket(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/token_plan/remains" {
			t.Errorf("path = %q", r.URL.Path)
		}
		var remaining int
		switch r.Header.Get("Authorization") {
		case "Bearer sk-cp-one":
			remaining = 80
		case "Bearer sk-cp-two":
			remaining = 25
		default:
			t.Errorf("unexpected authorization header")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"base_resp": map[string]any{"status_code": 0},
			"model_remains": []map[string]any{
				{"model_name": "image-01", "current_interval_remaining_percent": 1},
				{"model_name": "MiniMax-M*", "current_interval_remaining_percent": remaining,
					"current_weekly_remaining_percent": 60, "end_time": 1_800_000_000_000, "weekly_end_time": 1_800_300_000_000},
			},
		})
	}))
	defer server.Close()
	source := planKeySource{
		keys:     map[string]string{"one": "sk-cp-one", "two": "sk-cp-two"},
		accounts: []agentauth.Account{{ID: "one"}, {ID: "two"}},
	}
	provider := &Provider{apiKeys: source, planUsage: &apiKeyValidator{client: server.Client(), endpoint: server.URL + "/v1/token_plan/remains"}}
	got := provider.ReadPlanUsage(context.Background())
	if len(got) != 2 || got[0].AccountID != "one" || got[1].AccountID != "two" {
		t.Fatalf("account readings = %#v", got)
	}
	for i, expected := range []float64{20, 75} {
		if got[i].Err != nil || len(got[i].Windows) != 2 {
			t.Fatalf("account %d = %#v", i, got[i])
		}
		if got[i].Windows[0].UsedPercent == nil || *got[i].Windows[0].UsedPercent != expected ||
			got[i].Windows[0].ResetsAt != 1_800_000_000 || got[i].Windows[1].ResetsAt != 1_800_300_000 {
			t.Fatalf("account %d windows = %#v", i, got[i].Windows)
		}
	}
}

func TestMiniMaxUsesExplicitPercentagesAndOnlyReportedWindows(t *testing.T) {
	rows := []json.RawMessage{
		json.RawMessage(`{"model_name":"general","current_interval_total_count":1500,"current_interval_usage_count":1497,"current_interval_remaining_percent":0,"remains_time":60000}`),
	}
	now := time.Unix(1_800_000_000, 0)
	windows, err := miniMaxPlanWindows(rows, now)
	if err != nil || len(windows) != 1 || windows[0].UsedPercent == nil || *windows[0].UsedPercent != 100 ||
		windows[0].ResetsAt != now.Unix()+60 {
		t.Fatalf("windows = %#v, err = %v", windows, err)
	}
	if windows[0].Window != agent.QuotaWindowSession {
		t.Fatalf("window = %q", windows[0].Window)
	}
	if _, err := miniMaxPlanWindows([]json.RawMessage{json.RawMessage(`{"model_name":"general","current_interval_total_count":1500}`)}, now); err == nil {
		t.Fatal("count-only response was presented as a percentage")
	}
}

func TestMiniMaxLegacyKeyAndAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"secret":"do not expose"}`))
	}))
	defer server.Close()
	provider := &Provider{
		apiKeys:   planKeySource{keys: map[string]string{"": "sk-cp-old"}},
		planUsage: &apiKeyValidator{client: server.Client(), endpoint: server.URL},
	}
	got := provider.ReadPlanUsage(context.Background())
	if len(got) != 1 || got[0].AccountID != "" || !errors.Is(got[0].Err, agentauth.ErrAPIKeyRejected) {
		t.Fatalf("legacy reading = %#v", got)
	}
	if strings.Contains(got[0].Err.Error(), "do not expose") || len(got[0].Windows) != 0 {
		t.Fatal("provider response leaked into quota output")
	}
}
