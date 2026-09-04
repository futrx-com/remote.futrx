package httphandlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	agentauth "github.com/futrx-com/remote.futrx.com/internal/service/agent/auth"
	agentmodule "github.com/futrx-com/remote.futrx.com/internal/service/agent/module"
)

const missingAgentCLI = "futrx-test-agent-cli-that-does-not-exist"

var (
	testCodeRequired = errors.New("code is required")
	testNoSession    = errors.New("no login session in progress - call /api/claude/login/start first")
)

type agentAuthDeviceStatus struct {
	Authenticated bool                  `json:"authenticated"`
	DeviceLogin   agentauth.DeviceState `json:"deviceLogin,omitempty"`
}

type agentAuthTestModules []agentmodule.Descriptor

func (m agentAuthTestModules) Descriptors() []agentmodule.Descriptor {
	return append([]agentmodule.Descriptor(nil), m...)
}

func TestAgentAuthCatalogPublishesOrderedFactoryMetadata(t *testing.T) {
	handler := newTestAgentAuthHandler()
	handler.modules = agentAuthTestModules{
		{
			ID: "future-agent", Label: "Future Agent", Default: true,
			ExecutionScopes: []agentmodule.ExecutionScope{agentmodule.ScopeHost},
			Auth:            agentmodule.AuthExternal, AuthInstructions: "Run future-agent login.",
		},
		{
			ID: agent.ProviderCodex, Label: "Codex",
			ExecutionScopes: []agentmodule.ExecutionScope{agentmodule.ScopeHost, agentmodule.ScopeProject},
			Auth:            agentmodule.AuthManagedDevice, AuthInstructions: "Complete device login.",
			SatisfiesAccessGate: true,
		},
		{
			ID: agent.ProviderMiniMax, Label: "MiniMax",
			ExecutionScopes: []agentmodule.ExecutionScope{agentmodule.ScopeProject},
			Auth:            agentmodule.AuthManagedAPIKey, AuthInstructions: "Add an API key.",
			APIKeyAuth: &agentmodule.APIKeyAuth{
				CreateURL:       "https://platform.minimax.io/subscribe/token-plan",
				CreateLabel:     "Get a MiniMax Token Plan subscription key",
				CredentialLabel: "MiniMax Token Plan subscription key",
			},
		},
	}
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/agent-auth", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var response agentAuthCatalogResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Providers) != 3 || response.Providers[0].Provider != "future-agent" ||
		!response.Providers[0].Default || response.Providers[0].Authentication.Mode != agentmodule.AuthExternal {
		t.Fatalf("providers = %#v", response.Providers)
	}
	codex := response.Providers[1]
	if codex.Provider != "codex" || codex.Authentication.Mode != agentmodule.AuthManagedDevice ||
		!codex.Authentication.SatisfiesAccessGate || codex.Status.Authenticated {
		t.Fatalf("codex = %#v", codex)
	}
	miniMax := response.Providers[2]
	if miniMax.Authentication.Mode != agentmodule.AuthManagedAPIKey ||
		miniMax.Authentication.APIKey == nil ||
		miniMax.Authentication.APIKey.CreateURL != "https://platform.minimax.io/subscribe/token-plan" ||
		miniMax.Authentication.APIKey.CreateLabel != "Get a MiniMax Token Plan subscription key" ||
		miniMax.Authentication.APIKey.CredentialLabel != "MiniMax Token Plan subscription key" {
		t.Fatalf("minimax = %#v", miniMax)
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/agent-auth", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestAgentAuthStatusRoutesPreserveProviderPayloads(t *testing.T) {
	handler := newTestAgentAuthHandler()
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	tests := []struct {
		path string
		want string
	}{
		{path: "/api/claude/auth-status", want: `{"authenticated":false,"login":{"active":false}}` + "\n"},
		{path: "/api/codex/auth-status", want: `{"authenticated":false,"deviceLogin":{"active":false}}` + "\n"},
		{path: "/api/kimi/auth-status", want: `{"authenticated":false,"deviceLogin":{"active":false}}` + "\n"},
	}

	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodDelete, test.path, nil)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK || rec.Body.String() != test.want {
				t.Fatalf("response = %d %q, want %d %q", rec.Code, rec.Body.String(), http.StatusOK, test.want)
			}
			if got := rec.Header().Get("Content-Type"); got != "application/json" {
				t.Fatalf("Content-Type = %q, want application/json", got)
			}
		})
	}
}

func TestAgentAuthMutationRoutesRemainPostOnly(t *testing.T) {
	handler := newTestAgentAuthHandler()
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	paths := []string{
		"/api/claude/login/start",
		"/api/claude/login/code",
		"/api/claude/login/cancel",
		"/api/codex/login/device",
		"/api/kimi/login/device",
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
			if rec.Code != http.StatusMethodNotAllowed || rec.Body.String() != `{"error":"method not allowed"}`+"\n" {
				t.Fatalf("response = %d %q", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestAgentAuthAPIKeyRouteSavesAndDeletesWithoutReturningCredential(t *testing.T) {
	handler := newTestAgentAuthHandler()
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(
		http.MethodPost,
		"/api/minimax/login/api-key",
		strings.NewReader(`{"apiKey":" secret-key "}`),
	))
	if rec.Code != http.StatusOK || rec.Body.String() != `{"authenticated":true,"login":{"active":false}}`+"\n" {
		t.Fatalf("save response = %d %q", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "secret-key") {
		t.Fatal("API key leaked in response")
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/api/minimax/login/api-key", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != `{"authenticated":false,"login":{"active":false}}`+"\n" {
		t.Fatalf("delete response = %d %q", rec.Code, rec.Body.String())
	}
}

func TestAgentAuthAPIKeyRouteValidatesMethodAndKey(t *testing.T) {
	handler := newTestAgentAuthHandler()
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	tests := []struct {
		method string
		body   string
		want   int
	}{
		{method: http.MethodGet, want: http.StatusMethodNotAllowed},
		{method: http.MethodPost, body: `{"apiKey":" "}`, want: http.StatusBadRequest},
	}
	for _, test := range tests {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(test.method, "/api/minimax/login/api-key", strings.NewReader(test.body)))
		if rec.Code != test.want {
			t.Fatalf("%s response = %d %q, want %d", test.method, rec.Code, rec.Body.String(), test.want)
		}
	}
}

func TestAgentAuthAPIKeyRouteMapsProviderRejectionToCallerError(t *testing.T) {
	apiKeys, err := agentauth.NewAPIKeyService(
		context.Background(),
		agent.ProviderMiniMax,
		&agentAuthMemoryKeyStore{keys: map[agent.ProviderID]string{}},
		agentauth.APIKeyValidatorFunc(func(context.Context, string) error {
			return agentauth.ErrAPIKeyRejected
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	handler := NewAgentAuthHandler([]agentauth.Binding{
		agentauth.NewAPIKeyBinding(agent.ProviderMiniMax, apiKeys),
	}, nil)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(
		http.MethodPost,
		"/api/minimax/login/api-key",
		strings.NewReader(`{"apiKey":"rejected-key"}`),
	))
	if rec.Code != http.StatusBadRequest ||
		rec.Body.String() != `{"error":"API key is invalid or unauthorized"}`+"\n" {
		t.Fatalf("response = %d %q", rec.Code, rec.Body.String())
	}
}

func TestAgentAuthCodeErrorsKeepTheirHTTPMapping(t *testing.T) {
	tests := []struct {
		name string
		path string
		body string
		want int
		text string
	}{
		{
			name: "malformed json",
			path: "/api/claude/login/code",
			body: `{`,
			want: http.StatusBadRequest,
			text: `{"error":"invalid json: unexpected EOF"}` + "\n",
		},
		{
			name: "blank code",
			path: "/api/claude/login/code",
			body: `{"code":"  "}`,
			want: http.StatusBadRequest,
			text: `{"error":"code is required"}` + "\n",
		},
		{
			name: "missing session",
			path: "/api/claude/login/code",
			body: `{"code":"abc"}`,
			want: http.StatusBadRequest,
			text: `{"error":"no login session in progress - call /api/claude/login/start first"}` + "\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handler := newTestAgentAuthHandler()
			mux := http.NewServeMux()
			handler.RegisterRoutes(mux)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, test.path, strings.NewReader(test.body)))
			if rec.Code != test.want || rec.Body.String() != test.text {
				t.Fatalf("response = %d %q, want %d %q", rec.Code, rec.Body.String(), test.want, test.text)
			}
		})
	}
}

func TestAgentAuthOperationalErrorsAndCancelShape(t *testing.T) {
	handler := newTestAgentAuthHandler()
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	tests := []struct {
		path string
		want int
		body string
	}{
		{
			path: "/api/claude/login/start",
			want: http.StatusInternalServerError,
			body: `{"error":"claude CLI not found on PATH - install it first"}` + "\n",
		},
		{
			path: "/api/codex/login/device",
			want: http.StatusInternalServerError,
			body: `{"error":"codex CLI not found on PATH - install it first"}` + "\n",
		},
		{
			path: "/api/kimi/login/device",
			want: http.StatusInternalServerError,
			body: `{"error":"kimi CLI not found on PATH - install it first"}` + "\n",
		},
		{
			path: "/api/claude/login/cancel",
			want: http.StatusOK,
			body: `{"ok":true}` + "\n",
		},
	}

	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, test.path, nil))
			if rec.Code != test.want || rec.Body.String() != test.body {
				t.Fatalf("response = %d %q, want %d %q", rec.Code, rec.Body.String(), test.want, test.body)
			}
		})
	}
}

func newTestAgentAuthHandler() *AgentAuthHandler {
	code := agentauth.NewCodeService(agentauth.CodeConfig{
		Command:       missingAgentCLI,
		Authenticated: func() bool { return false },
		NotFound:      errors.New("claude CLI not found on PATH - install it first"),
		CodeRequired:  testCodeRequired,
		NoSession:     testNoSession,
	})
	device := func(notFound string) *agentauth.DeviceService[agentAuthDeviceStatus] {
		return agentauth.NewDeviceService(agentauth.DeviceConfig[agentAuthDeviceStatus]{
			Command:  missingAgentCLI,
			NotFound: errors.New(notFound),
			BuildStatus: func() agentauth.DeviceStatusBuilder[agentAuthDeviceStatus] {
				return func(state agentauth.DeviceState) agentAuthDeviceStatus {
					return agentAuthDeviceStatus{DeviceLogin: state}
				}
			},
		})
	}

	apiKeys, err := agentauth.NewAPIKeyService(context.Background(), agent.ProviderMiniMax, &agentAuthMemoryKeyStore{
		keys: map[agent.ProviderID]string{},
	}, nil)
	if err != nil {
		panic(err)
	}

	return NewAgentAuthHandler([]agentauth.Binding{
		agentauth.NewCodeBinding(agent.ProviderClaude, code),
		agentauth.NewDeviceBinding(agent.ProviderCodex, device("codex CLI not found on PATH - install it first")),
		agentauth.NewDeviceBinding(agent.ProviderKimi, device("kimi CLI not found on PATH - install it first")),
		agentauth.NewAPIKeyBinding(agent.ProviderMiniMax, apiKeys),
	}, nil)
}

type agentAuthMemoryKeyStore struct {
	keys map[agent.ProviderID]string
}

func (s *agentAuthMemoryKeyStore) AgentAPIKey(_ context.Context, id agent.ProviderID) (string, error) {
	return s.keys[id], nil
}

func (s *agentAuthMemoryKeyStore) SaveAgentAPIKey(_ context.Context, id agent.ProviderID, key string) error {
	s.keys[id] = key
	return nil
}

func (s *agentAuthMemoryKeyStore) DeleteAgentAPIKey(_ context.Context, id agent.ProviderID) error {
	delete(s.keys, id)
	return nil
}
