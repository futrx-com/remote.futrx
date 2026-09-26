package httphandlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	containerapps "github.com/futrx-com/remote.futrx.com/internal/integration/containers/applications"
	serviceapps "github.com/futrx-com/remote.futrx.com/internal/service/applications"
	serviceauth "github.com/futrx-com/remote.futrx.com/internal/service/auth"
	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
	"github.com/futrx-com/remote.futrx.com/internal/stores/fileapplications"
	"github.com/futrx-com/remote.futrx.com/internal/stores/fileproject"
)

func TestCodeServerRouteRequiresRunningProjectInstallation(t *testing.T) {
	ctx := context.Background()
	handler, _ := newVerifyHandler(t)
	projectStore, err := fileproject.NewWithWorkspaceRoot(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	handler.codeServer.projects = serviceproject.New(projectStore, serviceproject.ContainerDependencies{}, nil, nil)
	project, err := handler.codeServer.projects.Create(ctx, serviceproject.CreateInput{Name: "IDE Project"}, "user@example.com")
	if err != nil {
		t.Fatal(err)
	}
	appStore, err := fileapplications.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	registry, err := containerapps.NewRegistry(containerapps.EmbeddedCatalog(), nil)
	if err != nil {
		t.Fatal(err)
	}
	handler.codeServer.applications = serviceapps.New(registry, appStore, nil, nil, nil)

	if got := handler.codeServerSlug("code."+verifyBaseHost, "/"+project.Slug+"/?folder=/workspace"); got != project.Slug {
		t.Fatalf("launcher route slug = %q", got)
	}
	if got := handler.codeServerSlug(verifyBaseHost, "/"+project.Slug+"/code/?folder=/workspace"); got != project.Slug {
		t.Fatalf("main-site route slug = %q", got)
	}
	if got := handler.codeServerSlug(verifyBaseHost, "/"+project.Slug+"/code-other"); got != "" {
		t.Fatalf("non-IDE route accepted: %q", got)
	}
	if got := handler.codeServerSlug(project.Slug+".code."+verifyBaseHost, "/"); got != project.Slug {
		t.Fatalf("project subdomain slug = %q", got)
	}
	if got := handler.codeServerSlug("code."+verifyBaseHost+".evil.test", "/"+project.Slug+"/"); got != "" {
		t.Fatalf("foreign host accepted: %q", got)
	}

	available, err := handler.codeServer.available(ctx, project.Slug)
	if err != nil || available {
		t.Fatalf("uninstalled app available = %v, err = %v", available, err)
	}
	inst := serviceapps.Instance{
		ID: "ide-1", ApplicationID: "code-server", Scope: serviceapps.ScopeProject,
		ProjectID: string(project.ID), Status: serviceapps.StatusRunning,
	}
	if err := appStore.Put(ctx, inst); err != nil {
		t.Fatal(err)
	}
	available, err = handler.codeServer.available(ctx, project.Slug)
	if err != nil || !available {
		t.Fatalf("running app available = %v, err = %v", available, err)
	}
	inst.Status = serviceapps.StatusStopped
	if err := appStore.Put(ctx, inst); err != nil {
		t.Fatal(err)
	}
	available, err = handler.codeServer.available(ctx, project.Slug)
	if err != nil || available {
		t.Fatalf("stopped app available = %v, err = %v", available, err)
	}
}

func TestCodeServerForwardAuthRejectsAProjectNonmember(t *testing.T) {
	handler, _ := newVerifyHandler(t)
	cookie, err := handler.auth.IssueSession(context.Background(),
		serviceauth.User{Email: "nonmember@example.test", Sub: "google-1"},
		serviceauth.SignInMethodGoogle, "127.0.0.1", "test")
	if err != nil {
		t.Fatal(err)
	}
	for _, route := range []struct{ host, uri string }{
		{verifyBaseHost, "/" + verifyProjectSlug + "/code/?folder=/workspace"},
		{verifyProjectSlug + ".code." + verifyBaseHost, "/"},
		{"code." + verifyBaseHost, "/" + verifyProjectSlug + "/?folder=/workspace"},
	} {
		req := httptest.NewRequest(http.MethodGet, "/auth/verify-code-server", nil)
		req.Header.Set("X-Forwarded-Host", route.host)
		req.Header.Set("X-Forwarded-Uri", route.uri)
		req.AddCookie(&http.Cookie{Name: serviceauth.SessionCookieName, Value: cookie})
		rec := httptest.NewRecorder()
		handler.verifyCodeServer(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s%s: status %d, want 403", route.host, route.uri, rec.Code)
		}
	}
}
