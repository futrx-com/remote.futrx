package httphandlers

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/rbac"
	"github.com/futrx-com/remote.futrx.com/internal/service/harness"
	accountaccess "github.com/futrx-com/remote.futrx.com/internal/service/harness/accountaccess"
	harnessauth "github.com/futrx-com/remote.futrx.com/internal/service/harness/auth"
	harnessmodule "github.com/futrx-com/remote.futrx.com/internal/service/harness/module"
	harnessquota "github.com/futrx-com/remote.futrx.com/internal/service/harness/quota"
	httpmiddleware "github.com/futrx-com/remote.futrx.com/internal/transport/http/middleware"
)

type permissionAccountSource struct{ harnessauth.AccountController }

func (permissionAccountSource) AccountsSnapshot() harnessauth.AccountsSnapshot {
	return harnessauth.AccountsSnapshot{ActiveAccountID: "private", Items: []harnessauth.Account{{ID: "private", Label: "private label", Email: "private@example.com"}, {ID: "team", Label: "Team"}}}
}

type permissionQuota struct{ provider harness.ProviderID }

func (permissionQuota) Refresh(context.Context) {}
func (q permissionQuota) View() []harnessquota.AccountView {
	return []harnessquota.AccountView{{AccountQuota: harnessquota.AccountQuota{Provider: string(q.provider), AccountID: "private"}}, {AccountQuota: harnessquota.AccountQuota{Provider: string(q.provider), AccountID: "team"}}}
}
func TestAccountMetadataRoutesFilterWithRealMiddlewareAndPolicy(t *testing.T) {
	for _, provider := range []harness.ProviderID{harness.ProviderClaude, harness.ProviderCodex, harness.ProviderMiniMax, harness.ProviderKimi, harness.ProviderAntigravity} {
		t.Run(string(provider), func(t *testing.T) { testAccountMetadataRoutes(t, provider) })
	}
}
func testAccountMetadataRoutes(t *testing.T, provider harness.ProviderID) {
	f := newMembershipRoutesFixture(t)
	binding := harnessauth.NewExternalBinding(provider).WithAccounts(permissionAccountSource{})
	access := accountaccess.New(f.permissions, []harnessauth.Binding{binding})
	mux := http.NewServeMux()
	NewAgentAuthHandler([]harnessauth.Binding{binding}, f.auth, agentAuthTestModules{{ID: provider, Label: string(provider), Auth: harnessmodule.AuthExternal}}).WithAccountAccess(access).RegisterRoutes(mux)
	NewAgentQuotaHandler(permissionQuota{provider: provider}, f.auth).WithAccountAccess(access).RegisterRoutes(mux)
	NewPermissionsHandler(f.permissions).WithAccounts(access).RegisterRoutes(mux)
	f.handler = httpmiddleware.NewAuth(f.auth).Wrap(mux)
	if _, err := f.permissions.SetAssignment(adminContext(), rbac.AssignmentInput{UserEmail: "member@example.com", Permission: harnessauth.PermissionAccountUse, Effect: rbac.Allow, Scope: rbac.ProviderAccountScope(string(provider), "team")}); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/api/agent-auth", "/api/agent-quota"} {
		r := f.do(t, "member@example.com", "GET", path, "")
		if r.Code != 200 || strings.Contains(r.Body.String(), "private") || !strings.Contains(r.Body.String(), "team") {
			t.Fatalf("%s: %d %s", path, r.Code, r.Body)
		}
	}
	for _, path := range []string{"/api/" + string(provider) + "/auth-status", "/api/admin/permissions/accounts"} {
		if r := f.do(t, "member@example.com", "GET", path, ""); r.Code != 403 {
			t.Fatalf("%s must require management: %d %s", path, r.Code, r.Body)
		}
	}
	if r := f.do(t, "admin@example.com", "GET", "/api/admin/permissions/accounts", ""); r.Code != 200 || !strings.Contains(r.Body.String(), "private label") {
		t.Fatalf("admin targets: %d %s", r.Code, r.Body)
	}
	if _, err := f.permissions.SetAssignment(adminContext(), rbac.AssignmentInput{UserEmail: "member@example.com", Permission: harnessauth.PermissionAccountUse, Effect: rbac.Deny, Scope: rbac.ProviderAccountScope(string(provider), "team")}); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/api/agent-auth", "/api/agent-quota"} {
		r := f.do(t, "member@example.com", "GET", path, "")
		if r.Code != 200 || strings.Contains(r.Body.String(), "team") {
			t.Fatalf("revoked %s: %d %s", path, r.Code, r.Body)
		}
	}
}
