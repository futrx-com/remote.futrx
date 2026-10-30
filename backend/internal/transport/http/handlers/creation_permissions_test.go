package httphandlers

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/rbac"
	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
	"github.com/futrx-com/remote.futrx.com/internal/stores/filechat"
	httpmiddleware "github.com/futrx-com/remote.futrx.com/internal/transport/http/middleware"
)

type creationProjectPath struct{ projects *serviceproject.Service }

func (p creationProjectPath) WorkspaceForProject(ctx context.Context, id servicechat.ProjectID) (string, error) {
	return p.projects.WorkspaceForProject(ctx, serviceproject.ID(id))
}

func TestHTTPProjectCreationDenialLeavesRepositoryUntouched(t *testing.T) {
	f := newMembershipRoutesFixture(t)
	allowed := f.do(t, "member@example.com", "POST", "/api/projects", `{"name":"Allowed project"}`)
	if allowed.Code != 201 {
		t.Fatalf("allowed: %d %s", allowed.Code, allowed.Body)
	}
	before, err := f.projects.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.permissions.SetAssignment(adminContext(), rbac.AssignmentInput{UserEmail: "member@example.com", Permission: serviceproject.PermissionCreate, Effect: rbac.Deny, Scope: rbac.PlatformScope()}); err != nil {
		t.Fatal(err)
	}
	denied := f.do(t, "member@example.com", "POST", "/api/projects", `{"name":"Denied project"}`)
	if denied.Code != 403 {
		t.Fatalf("denied: %d %s", denied.Code, denied.Body)
	}
	after, err := f.projects.List(context.Background())
	if err != nil || len(after) != len(before) {
		t.Fatalf("side effect: %v %v", after, err)
	}
}

func TestHTTPHostAndProjectChatCreationAndForkHonorPolicy(t *testing.T) {
	for _, projectScope := range []bool{false, true} {
		f := newMembershipRoutesFixture(t)
		store, err := filechat.New(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = store.Close() })
		chats := servicechat.New(store, creationProjectPath{f.projects}, nil, nil, servicechat.WithAuthorizer(f.permissions))
		mux := http.NewServeMux()
		NewChatHandler(chats, servicechat.NewAccessService(chats, f.projects), f.auth, nil, nil).RegisterRoutes(mux)
		f.handler = httpmiddleware.NewAuth(f.auth).Wrap(mux)
		input := servicechat.CreateInput{Title: "Allowed", Provider: servicechat.ProviderClaude}
		scope, key := rbac.PlatformScope(), servicechat.PermissionHostCreate
		if projectScope {
			input.ProjectID = servicechat.ProjectID(f.projectID)
			scope = rbac.ProjectScope(f.projectID)
			key = servicechat.PermissionProjectCreate
		}
		body, _ := json.Marshal(input)
		response := f.do(t, "member@example.com", "POST", "/api/chats", string(body))
		if response.Code != 201 {
			t.Fatalf("allowed create: %d %s", response.Code, response.Body)
		}
		var created servicechat.Meta
		if err = json.Unmarshal(response.Body.Bytes(), &created); err != nil {
			t.Fatal(err)
		}
		forkPath := "/api/chats/" + string(created.ID) + "/fork"
		response = f.do(t, "member@example.com", "POST", forkPath, "")
		if response.Code != 201 {
			t.Fatalf("allowed fork: %d %s", response.Code, response.Body)
		}
		before, err := chats.List(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if _, err = f.permissions.SetAssignment(adminContext(), rbac.AssignmentInput{UserEmail: "member@example.com", Permission: key, Effect: rbac.Deny, Scope: scope}); err != nil {
			t.Fatal(err)
		}
		for _, path := range []string{"/api/chats", forkPath} {
			response = f.do(t, "member@example.com", "POST", path, string(body))
			if response.Code != 403 {
				t.Fatalf("denied %s: %d %s", path, response.Code, response.Body)
			}
		}
		after, err := chats.List(context.Background())
		if err != nil || len(after) != len(before) {
			t.Fatalf("denied chat side effects: %v %v", after, err)
		}
	}
}
