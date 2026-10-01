package httphandlers

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	"github.com/futrx-com/remote.futrx.com/internal/rbac"
	accountaccess "github.com/futrx-com/remote.futrx.com/internal/service/agent/accountaccess"
	agentauth "github.com/futrx-com/remote.futrx.com/internal/service/agent/auth"
	agentmodule "github.com/futrx-com/remote.futrx.com/internal/service/agent/module"
	agentquota "github.com/futrx-com/remote.futrx.com/internal/service/agent/quota"
	httpmiddleware "github.com/futrx-com/remote.futrx.com/internal/transport/http/middleware"
)

type permissionAccountSource struct{ agentauth.AccountController }

func (permissionAccountSource) AccountsSnapshot() agentauth.AccountsSnapshot {
	return agentauth.AccountsSnapshot{ActiveAccountID: "private", Items: []agentauth.Account{{ID: "private", Label: "private label", Email: "private@example.com"}, {ID: "team", Label: "Team"}}}
}

type permissionQuota struct{}

func (permissionQuota) Refresh(context.Context) {}
func (permissionQuota) View() []agentquota.AccountView {
	return []agentquota.AccountView{{AccountQuota: agentquota.AccountQuota{Provider: "codex", AccountID: "private"}}, {AccountQuota: agentquota.AccountQuota{Provider: "codex", AccountID: "team"}}}
}
func TestAccountMetadataRoutesFilterWithRealMiddlewareAndPolicy(t *testing.T) {
	f := newMembershipRoutesFixture(t)
	binding := agentauth.NewExternalBinding(agent.ProviderCodex).WithAccounts(permissionAccountSource{})
	access := accountaccess.New(f.permissions, []agentauth.Binding{binding})
	mux := http.NewServeMux()
	NewAgentAuthHandler([]agentauth.Binding{binding}, f.auth, agentAuthTestModules{{ID: agent.ProviderCodex, Label: "Codex", Auth: agentmodule.AuthExternal}}).WithAccountAccess(access).RegisterRoutes(mux)
	NewAgentQuotaHandler(permissionQuota{}, f.auth).WithAccountAccess(access).RegisterRoutes(mux)
	NewPermissionsHandler(f.permissions).WithAccounts(access).RegisterRoutes(mux)
	f.handler = httpmiddleware.NewAuth(f.auth).Wrap(mux)
	if _, err := f.permissions.SetAssignment(adminContext(), rbac.AssignmentInput{UserEmail: "member@example.com", Permission: agentauth.PermissionAccountUse, Effect: rbac.Allow, Scope: rbac.ProviderAccountScope("codex", "team")}); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/api/agent-auth", "/api/agent-quota"} {
		r := f.do(t, "member@example.com", "GET", path, "")
		if r.Code != 200 || strings.Contains(r.Body.String(), "private") || !strings.Contains(r.Body.String(), "team") {
			t.Fatalf("%s: %d %s", path, r.Code, r.Body)
		}
	}
	for _, path := range []string{"/api/codex/auth-status", "/api/permissions/accounts"} {
		if r := f.do(t, "member@example.com", "GET", path, ""); r.Code != 403 {
			t.Fatalf("%s must require management: %d %s", path, r.Code, r.Body)
		}
	}
	if r := f.do(t, "admin@example.com", "GET", "/api/permissions/accounts", ""); r.Code != 200 || !strings.Contains(r.Body.String(), "private label") {
		t.Fatalf("admin targets: %d %s", r.Code, r.Body)
	}
	if _, err := f.permissions.SetAssignment(adminContext(), rbac.AssignmentInput{UserEmail: "member@example.com", Permission: agentauth.PermissionAccountUse, Effect: rbac.Deny, Scope: rbac.ProviderAccountScope("codex", "team")}); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/api/agent-auth", "/api/agent-quota"} {
		r := f.do(t, "member@example.com", "GET", path, "")
		if r.Code != 200 || strings.Contains(r.Body.String(), "team") {
			t.Fatalf("revoked %s: %d %s", path, r.Code, r.Body)
		}
	}
}
