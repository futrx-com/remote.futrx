package wstransport

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/rbac"
	"github.com/futrx-com/remote.futrx.com/internal/service/harness"
	accountaccess "github.com/futrx-com/remote.futrx.com/internal/service/harness/accountaccess"
	harnessauth "github.com/futrx-com/remote.futrx.com/internal/service/harness/auth"
	"github.com/gorilla/websocket"
)

type streamAccountPolicy struct {
	revoked  atomic.Bool
	provider harness.ProviderID
}

func (p *streamAccountPolicy) Require(ctx context.Context, check rbac.Check) error {
	actor, ok := rbac.ActorFromContext(ctx)
	if !ok || actor.Email != "member@example.com" {
		return rbac.ErrActorRequired
	}
	if check.Permission == harnessauth.PermissionAccountUse && check.Scope.ID == string(p.provider)+":team" && !p.revoked.Load() {
		return nil
	}
	return rbac.ErrDenied
}

type streamAccounts struct{ harnessauth.AccountController }

func (streamAccounts) AccountsSnapshot() harnessauth.AccountsSnapshot {
	return harnessauth.AccountsSnapshot{ActiveAccountID: "private", Items: []harnessauth.Account{{ID: "private", Label: "Private label"}, {ID: "team", Label: "Team"}}}
}
func TestAccountStatusStreamsFilterAndRefreshRevocationWithoutProviderEvents(t *testing.T) {
	for _, provider := range []harness.ProviderID{harness.ProviderClaude, harness.ProviderCodex, harness.ProviderMiniMax, harness.ProviderKimi} {
		t.Run(string(provider), func(t *testing.T) {
			t.Parallel()
			testAccountStatusStream(t, provider)
		})
	}
}
func testAccountStatusStream(t *testing.T, provider harness.ProviderID) {
	bindings, _ := newSocketTestBindings()
	for i := range bindings {
		if bindings[i].ID() == provider {
			bindings[i] = bindings[i].WithAccounts(streamAccounts{})
		}
	}
	policy := &streamAccountPolicy{provider: provider}
	access := accountaccess.New(policy, bindings)
	mux := http.NewServeMux()
	NewAgentAuthSocket(bindings).WithAccountAccess(access).RegisterRoutes(mux, websocket.Upgrader{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mux.ServeHTTP(w, r.WithContext(rbac.ContextWithActor(r.Context(), rbac.UserActor("member@example.com"))))
	}))
	defer server.Close()
	raw, response, err := websocket.DefaultDialer.Dial(webSocketURL(server.URL, "/ws/"+string(provider)+"/auth-status"), nil)
	if raw != nil {
		raw.Close()
	}
	if err == nil || response == nil || response.StatusCode != 403 {
		t.Fatalf("raw status must require management: %v %v", response, err)
	}
	conn, _, err := websocket.DefaultDialer.Dial(webSocketURL(server.URL, "/ws/agent-auth/"+string(provider)), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, payload, err := conn.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(payload), "private") || !strings.Contains(string(payload), "team") {
		t.Fatalf("filtered initial payload: %s", payload)
	}
	policy.revoked.Store(true)
	// No provider Broadcast: the periodic policy refresh must remove the account.
	_ = conn.SetReadDeadline(time.Now().Add(20 * time.Second))
	_, payload, err = conn.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	var snapshot harnessauth.Snapshot
	if err := json.Unmarshal(payload, &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.Accounts == nil || len(snapshot.Accounts.Items) != 0 || snapshot.Authenticated {
		t.Fatalf("revoked stream: %s", payload)
	}
}
