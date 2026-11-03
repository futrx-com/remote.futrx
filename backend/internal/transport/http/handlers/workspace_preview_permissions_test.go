package httphandlers

import (
	"context"
	"fmt"
	"github.com/futrx-com/remote.futrx.com/internal/rbac"
	serviceauth "github.com/futrx-com/remote.futrx.com/internal/service/auth"
	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
	"github.com/futrx-com/remote.futrx.com/internal/service/workspaceide"
	"net/http"
	"testing"
)

func TestWorkspacePreviewEnforcesCurrentPolicyAndRevocation(t *testing.T) {
	for _, tc := range []struct {
		port int
		key  rbac.Key
	}{{8842, workspaceide.PermissionProjectOpen}, {8843, "workspace.terminal.use"}, {6080, "workspace.browser.use"}, {9222, "workspace.browser.use"}} {
		t.Run(fmt.Sprint(tc.port), func(t *testing.T) {
			f := newMembershipRoutesFixture(t)
			project, err := f.projects.Get(context.Background(), serviceproject.ID(f.projectID))
			if err != nil {
				t.Fatal(err)
			}
			h := &authVerifyHandler{auth: f.auth, access: serviceauth.NewAccessVerifier(f.auth, f.projects).WithWorkspaceAuthorizer(f.permissions), shares: &shareAuthorizerStub{validToken: "shared", allows: true}}
			cookie := func(email string) *http.Cookie {
				value, err := f.auth.IssueSession(context.Background(), serviceauth.User{Email: email}, serviceauth.SignInMethodPassword, "", "")
				if err != nil {
					t.Fatal(err)
				}
				return &http.Cookie{Name: serviceauth.SessionCookieName, Value: value}
			}
			member, admin, outsider := cookie("member@example.com"), cookie("admin@example.com"), cookie("outsider@example.com")
			routes := []struct{ host, uri string }{
				{project.Slug + fmt.Sprint("--", tc.port, ".dev.") + verifyBaseHost, "/?share=shared"},
			}
			for _, route := range routes {
				for _, test := range []struct {
					cookie *http.Cookie
					status int
				}{{nil, 302}, {member, 200}, {admin, 200}, {outsider, 403}} {
					r := verifyRequest(t, h, route.host, route.uri, test.cookie)
					if r.Code != test.status {
						t.Fatalf("%s%s: %d want %d: %s", route.host, route.uri, r.Code, test.status, r.Body)
					}
				}
			}
			if _, err := f.permissions.SetAssignment(adminContext(), rbac.AssignmentInput{UserEmail: "member@example.com", Permission: tc.key, Effect: rbac.Deny, Scope: rbac.ProjectScope(f.projectID)}); err != nil {
				t.Fatal(err)
			}
			// Reuse the same session; revocation does not depend on signing in again.
			for _, route := range routes {
				if r := verifyRequest(t, h, route.host, route.uri, member); r.Code != 403 {
					t.Fatalf("revoked %s%s: %d %s", route.host, route.uri, r.Code, r.Body)
				}
				if r := verifyRequest(t, h, route.host, route.uri, admin); r.Code != 200 {
					t.Fatalf("admin %s: %d", route.host, r.Code)
				}
			}
			if r := verifyRequest(t, h, "missing-project--8842.dev."+verifyBaseHost, "/", member); r.Code != 404 {
				t.Fatalf("missing: %d", r.Code)
			}
			h.access = serviceauth.NewAccessVerifier(f.auth, f.projects)
			if r := verifyRequest(t, h, routes[0].host, routes[0].uri, member); r.Code != 403 {
				t.Fatalf("missing authorizer: %d", r.Code)
			}

		})
	}
}
