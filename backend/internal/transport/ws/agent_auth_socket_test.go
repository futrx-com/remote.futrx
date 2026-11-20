package wstransport

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/service/harness"
	harnessauth "github.com/futrx-com/remote.futrx.com/internal/service/harness/auth"
	"github.com/gorilla/websocket"
)

type socketDeviceStatus struct {
	Authenticated bool                    `json:"authenticated"`
	DeviceLogin   harnessauth.DeviceState `json:"deviceLogin,omitempty"`
}

func TestAgentAuthSocketInitialStatusPayloads(t *testing.T) {
	bindings, _ := newSocketTestBindings()
	mux := http.NewServeMux()
	NewAgentAuthSocket(bindings).RegisterRoutes(mux, websocket.Upgrader{})
	server := httptest.NewServer(mux)
	defer server.Close()

	tests := []struct {
		path string
		want string
	}{
		{path: "/ws/claude/auth-status", want: `{"authenticated":false,"login":{"active":false}}` + "\n"},
		{path: "/ws/codex/auth-status", want: `{"authenticated":false,"deviceLogin":{"active":false}}` + "\n"},
		{path: "/ws/kimi/auth-status", want: `{"authenticated":false,"deviceLogin":{"active":false}}` + "\n"},
		{path: "/ws/minimax/auth-status", want: `{"authenticated":false}` + "\n"},
	}

	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			conn, _, err := websocket.DefaultDialer.Dial(webSocketURL(server.URL, test.path), nil)
			if err != nil {
				t.Fatalf("Dial: %v", err)
			}
			defer conn.Close()
			_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
			_, payload, err := conn.ReadMessage()
			if err != nil {
				t.Fatalf("ReadMessage: %v", err)
			}
			if string(payload) != test.want {
				t.Fatalf("payload = %q, want %q", payload, test.want)
			}
		})
	}
}

func TestAgentAuthSocketStreamsBroadcasts(t *testing.T) {
	bindings, code := newSocketTestBindings()
	mux := http.NewServeMux()
	NewAgentAuthSocket(bindings).RegisterRoutes(mux, websocket.Upgrader{})
	server := httptest.NewServer(mux)
	defer server.Close()

	conn, _, err := websocket.DefaultDialer.Dial(webSocketURL(server.URL, "/ws/claude/auth-status"), nil)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, _, err := conn.ReadMessage(); err != nil {
		t.Fatalf("read initial status: %v", err)
	}

	code.Broadcast()
	_, payload, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read broadcast: %v", err)
	}
	want := `{"authenticated":false,"login":{"active":false}}` + "\n"
	if string(payload) != want {
		t.Fatalf("broadcast payload = %q, want %q", payload, want)
	}
}

func TestAgentAuthSocketExposesNormalizedProviderStream(t *testing.T) {
	bindings, _ := newSocketTestBindings()
	mux := http.NewServeMux()
	NewAgentAuthSocket(bindings).RegisterRoutes(mux, websocket.Upgrader{})
	server := httptest.NewServer(mux)
	defer server.Close()

	conn, _, err := websocket.DefaultDialer.Dial(webSocketURL(server.URL, "/ws/agent-auth/codex"), nil)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, payload, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}
	want := `{"authenticated":false,"login":{"active":false}}` + "\n"
	if string(payload) != want {
		t.Fatalf("normalized payload = %q, want %q", payload, want)
	}
}

func TestAgentAuthSocketUnavailableRoutesKeepProviderError(t *testing.T) {
	bindings := []harnessauth.Binding{
		harnessauth.NewCodeBinding(harness.ProviderClaude, nil),
		harnessauth.NewDeviceBinding[socketDeviceStatus](harness.ProviderCodex, nil),
		harnessauth.NewDeviceBinding[socketDeviceStatus](harness.ProviderKimi, nil),
	}
	mux := http.NewServeMux()
	NewAgentAuthSocket(bindings).RegisterRoutes(mux, websocket.Upgrader{})

	for _, id := range []harness.ProviderID{harness.ProviderClaude, harness.ProviderCodex, harness.ProviderKimi} {
		t.Run(string(id), func(t *testing.T) {
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/ws/"+string(id)+"/auth-status", nil))
			want := string(id) + " auth stream unavailable\n"
			if rec.Code != http.StatusServiceUnavailable || rec.Body.String() != want {
				t.Fatalf("response = %d %q, want %d %q", rec.Code, rec.Body.String(), http.StatusServiceUnavailable, want)
			}
		})
	}
}

func newSocketTestBindings() ([]harnessauth.Binding, *harnessauth.CodeService) {
	code := harnessauth.NewCodeService(harnessauth.CodeConfig{Authenticated: func() bool { return false }})
	device := func() *harnessauth.DeviceService[socketDeviceStatus] {
		return harnessauth.NewDeviceService(harnessauth.DeviceConfig[socketDeviceStatus]{
			Authenticated: func() bool { return false },
			BuildStatus: func() harnessauth.DeviceStatusBuilder[socketDeviceStatus] {
				return func(state harnessauth.DeviceState) socketDeviceStatus {
					return socketDeviceStatus{DeviceLogin: state}
				}
			},
			NotFound: errors.New("not found"),
		})
	}
	apiKeys, err := harnessauth.NewAPIKeyService(context.Background(), harness.ProviderMiniMax, socketAPIKeyStore{}, nil, nil)
	if err != nil {
		panic(err)
	}
	return []harnessauth.Binding{
		harnessauth.NewCodeBinding(harness.ProviderClaude, code),
		harnessauth.NewDeviceBinding(harness.ProviderCodex, device()),
		harnessauth.NewDeviceBinding(harness.ProviderKimi, device()),
		harnessauth.NewAPIKeyBinding(harness.ProviderMiniMax, apiKeys),
	}, code
}

type socketAPIKeyStore struct{}

func (socketAPIKeyStore) HarnessAPIKey(context.Context, harness.ProviderID) (string, error) {
	return "", nil
}

func (socketAPIKeyStore) SaveHarnessAPIKey(context.Context, harness.ProviderID, string) error {
	return nil
}

func (socketAPIKeyStore) DeleteHarnessAPIKey(context.Context, harness.ProviderID) error {
	return nil
}

func webSocketURL(serverURL, path string) string {
	return "ws" + strings.TrimPrefix(serverURL, "http") + path
}
