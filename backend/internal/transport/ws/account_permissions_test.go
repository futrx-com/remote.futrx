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
	accountaccess "github.com/futrx-com/remote.futrx.com/internal/service/agent/accountaccess"
	agentauth "github.com/futrx-com/remote.futrx.com/internal/service/agent/auth"
	"github.com/gorilla/websocket"
)

type streamAccountPolicy struct{ revoked atomic.Bool }

func (p *streamAccountPolicy) Require(ctx context.Context, check rbac.Check) error {
	actor, ok := rbac.ActorFromContext(ctx)
	if !ok || actor.Email != "member@example.com" {
		return rbac.ErrActorRequired
	}
	if check.Permission == agentauth.PermissionAccountUse && check.Scope.ID == "claude:team" && !p.revoked.Load() {
		return nil
	}
	return rbac.ErrDenied
}

type streamAccounts struct{ agentauth.AccountController }

func (streamAccounts) AccountsSnapshot() agentauth.AccountsSnapshot {
	return agentauth.AccountsSnapshot{ActiveAccountID: "private", Items: []agentauth.Account{{ID: "private", Label: "Private label"}, {ID: "team", Label: "Team"}}}
}
func TestAccountStatusStreamsFilterAndRefreshRevocationWithoutProviderEvents(t *testing.T) {
	bindings, _ := newSocketTestBindings()
	bindings[0] = bindings[0].WithAccounts(streamAccounts{})
	policy := &streamAccountPolicy{}
	access := accountaccess.New(policy, bindings)
	mux := http.NewServeMux()
	NewAgentAuthSocket(bindings).WithAccountAccess(access).RegisterRoutes(mux, websocket.Upgrader{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mux.ServeHTTP(w, r.WithContext(rbac.ContextWithActor(r.Context(), rbac.UserActor("member@example.com"))))
	}))
	defer server.Close()
	raw, response, err := websocket.DefaultDialer.Dial(webSocketURL(server.URL, "/ws/claude/auth-status"), nil)
	if raw != nil {
		raw.Close()
	}
	if err == nil || response == nil || response.StatusCode != 403 {
		t.Fatalf("raw status must require management: %v %v", response, err)
	}
	conn, _, err := websocket.DefaultDialer.Dial(webSocketURL(server.URL, "/ws/agent-auth/claude"), nil)
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
	var snapshot agentauth.Snapshot
	if err := json.Unmarshal(payload, &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.Accounts == nil || len(snapshot.Accounts.Items) != 0 || snapshot.Authenticated {
		t.Fatalf("revoked stream: %s", payload)
	}
}
