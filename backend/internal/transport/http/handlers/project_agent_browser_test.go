package httphandlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
	"github.com/futrx-com/remote.futrx.com/internal/stores/fileproject"
)

func TestProjectAgentBrowserRoutes(t *testing.T) {
	handler, containers, project := newAgentBrowserProjectHandler(t)

	statusReq := httptest.NewRequest(http.MethodGet, "/api/projects/"+string(project.ID)+"/agent-browser", nil)
	statusReq.Host = "remote.futrx.com"
	statusRec := httptest.NewRecorder()
	handler.HandleResource(statusRec, statusReq)
	if statusRec.Code != http.StatusOK {
		t.Fatalf("GET status = %d body=%s", statusRec.Code, statusRec.Body.String())
	}
	var status agentBrowserResponse
	if err := json.NewDecoder(statusRec.Body).Decode(&status); err != nil {
		t.Fatal(err)
	}
	if status.Status != serviceproject.AgentBrowserStatusStopped || status.URL != "" || status.Slug != project.Slug || status.Port != 6080 {
		t.Fatalf("GET response = %#v", status)
	}

	startReq := httptest.NewRequest(http.MethodPost, "/api/projects/"+string(project.ID)+"/agent-browser/start", strings.NewReader("{}"))
	startReq.Host = "remote.futrx.com"
	startReq.Header.Set("X-Forwarded-Proto", "https")
	startRec := httptest.NewRecorder()
	handler.HandleResource(startRec, startReq)
	if startRec.Code != http.StatusOK {
		t.Fatalf("POST start = %d body=%s", startRec.Code, startRec.Body.String())
	}
	var started agentBrowserResponse
	if err := json.NewDecoder(startRec.Body).Decode(&started); err != nil {
		t.Fatal(err)
	}
	wantURL := "https://" + project.Slug + "--6080.dev.remote.futrx.com/vnc.html?autoconnect=1&resize=scale&reconnect=1"
	if started.Status != serviceproject.AgentBrowserStatusStarting || started.URL != "" || started.Slug != project.Slug || started.Port != 6080 {
		t.Fatalf("POST response = %#v", started)
	}
	containers.waitForAgentBrowserStart(t)

	statusReq = httptest.NewRequest(http.MethodGet, "/api/projects/"+string(project.ID)+"/agent-browser", nil)
	statusReq.Host = "remote.futrx.com"
	statusRec = httptest.NewRecorder()
	handler.HandleResource(statusRec, statusReq)
	if statusRec.Code != http.StatusOK {
		t.Fatalf("GET starting status = %d body=%s", statusRec.Code, statusRec.Body.String())
	}
	if err := json.NewDecoder(statusRec.Body).Decode(&status); err != nil {
		t.Fatal(err)
	}
	if status.Status != serviceproject.AgentBrowserStatusStarting || status.URL != "" {
		t.Fatalf("GET starting response = %#v", status)
	}

	containers.completeAgentBrowserStart()
	status = waitForAgentBrowserReady(t, handler, project)
	if status.Status != serviceproject.AgentBrowserStatusReady || status.URL != wantURL || status.Slug != project.Slug || status.Port != 6080 {
		t.Fatalf("GET ready response = %#v, want url %q", status, wantURL)
	}

	stopViewReq := httptest.NewRequest(http.MethodDelete, "/api/projects/"+string(project.ID)+"/agent-browser?scope=view", nil)
	stopViewRec := httptest.NewRecorder()
	handler.HandleResource(stopViewRec, stopViewReq)
	if stopViewRec.Code != http.StatusOK {
		t.Fatalf("DELETE stop view = %d body=%s", stopViewRec.Code, stopViewRec.Body.String())
	}
	if !containers.agentBrowserViewStopped() {
		t.Fatal("expected container Agent Browser view stop")
	}
	if err := json.NewDecoder(stopViewRec.Body).Decode(&status); err != nil {
		t.Fatal(err)
	}
	if status.Status != serviceproject.AgentBrowserStatusCoreReady || status.URL != "" || status.Core != "ready" || status.View != "off" {
		t.Fatalf("DELETE view response = %#v", status)
	}

	stopReq := httptest.NewRequest(http.MethodDelete, "/api/projects/"+string(project.ID)+"/agent-browser", nil)
	stopRec := httptest.NewRecorder()
	handler.HandleResource(stopRec, stopReq)
	if stopRec.Code != http.StatusOK {
		t.Fatalf("DELETE stop = %d body=%s", stopRec.Code, stopRec.Body.String())
	}
	if !containers.agentBrowserStopped() {
		t.Fatal("expected container Agent Browser stop")
	}
	var stopped map[string]serviceproject.AgentBrowserStatus
	if err := json.NewDecoder(stopRec.Body).Decode(&stopped); err != nil {
		t.Fatal(err)
	}
	if stopped["status"] != serviceproject.AgentBrowserStatusStopped {
		t.Fatalf("DELETE response = %#v", stopped)
	}
}

func TestProjectAgentBrowserRouteMethods(t *testing.T) {
	handler, _, project := newAgentBrowserProjectHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/api/projects/"+string(project.ID)+"/agent-browser", strings.NewReader("{}"))
	rec := httptest.NewRecorder()
	handler.HandleResource(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST /agent-browser status = %d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/projects/"+string(project.ID)+"/agent-browser/start", nil)
	rec = httptest.NewRecorder()
	handler.HandleResource(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("GET /agent-browser/start status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestProjectResourceLimitsRoute(t *testing.T) {
	handler, containers, project := newAgentBrowserProjectHandler(t)

	req := httptest.NewRequest(http.MethodPut, "/api/projects/"+string(project.ID)+"/limits", strings.NewReader(`{"cpu":"4","memory":"8GiB","disk":"40GiB"}`))
	rec := httptest.NewRecorder()
	handler.HandleResource(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT limits = %d body=%s", rec.Code, rec.Body.String())
	}
	want := serviceproject.ContainerLimits{CPU: "4", Memory: "8GiB", Disk: "40GiB"}
	if got := containers.currentResourceLimits(); got != want {
		t.Fatalf("applied limits = %#v, want %#v", got, want)
	}
	meta, err := handler.projects.Get(context.Background(), project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if meta.ResourceLimits == nil || *meta.ResourceLimits != want {
		t.Fatalf("persisted limits = %#v, want %#v", meta.ResourceLimits, want)
	}

	req = httptest.NewRequest(http.MethodPut, "/api/projects/"+string(project.ID)+"/limits", strings.NewReader(`{"cpu":"0"}`))
	rec = httptest.NewRecorder()
	handler.HandleResource(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("PUT invalid limits = %d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPut, "/api/projects/"+string(project.ID)+"/limits", strings.NewReader(`{}`))
	rec = httptest.NewRecorder()
	handler.HandleResource(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT reset limits = %d body=%s", rec.Code, rec.Body.String())
	}
	meta, err = handler.projects.Get(context.Background(), project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if meta.ResourceLimits != nil {
		t.Fatalf("persisted reset limits = %#v, want nil", meta.ResourceLimits)
	}
}

func newAgentBrowserProjectHandler(t *testing.T) (*ProjectHandler, *fakeProjectContainers, serviceproject.Meta) {
	t.Helper()
	repo, err := fileproject.NewWithWorkspaceRoot(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	containers := newFakeProjectContainers()
	projects := serviceproject.New(repo, serviceproject.ContainerDependencies{
		Lifecycle:   containers,
		Environment: containers,
		Inspector:   containers,
		Network:     containers,
		Listeners:   containers,
		Browser:     fakeProjectBrowser{containers: containers},
	}, nil, nil)
	project, err := projects.Create(context.Background(), serviceproject.CreateInput{Name: "Browser Project"}, "user@example.com")
	if err != nil {
		t.Fatal(err)
	}
	return NewProjectHandler(projects, nil, nil, nil, "remote.futrx.com"), containers, project
}

type fakeProjectContainers struct {
	mu                          sync.Mutex
	agentBrowserRunning         bool
	agentBrowserViewRunning     bool
	agentBrowserStarted         bool
	agentBrowserStoppedFlag     bool
	agentBrowserViewStoppedFlag bool
	agentBrowserStartedOnce     sync.Once
	agentBrowserAllowOnce       sync.Once
	agentBrowserStartedCh       chan struct{}
	agentBrowserAllowCh         chan struct{}
	resourceLimits              serviceproject.ContainerLimits
}

func newFakeProjectContainers() *fakeProjectContainers {
	return &fakeProjectContainers{
		agentBrowserStartedCh: make(chan struct{}),
		agentBrowserAllowCh:   make(chan struct{}),
	}
}

type fakeProjectBrowser struct {
	containers *fakeProjectContainers
}

func (f fakeProjectBrowser) Ensure(ctx context.Context, containerName string) error {
	return f.containers.ensureBrowser(ctx, containerName)
}

func (f fakeProjectBrowser) Stop(ctx context.Context, containerName string) error {
	return f.containers.stopBrowser(ctx, containerName)
}

func (f fakeProjectBrowser) StopView(ctx context.Context, containerName string) error {
	return f.containers.stopBrowserView(ctx, containerName)
}

func (f fakeProjectBrowser) Status(ctx context.Context, containerName string) (serviceproject.AgentBrowserInfo, error) {
	return f.containers.browserStatus(ctx, containerName)
}

func (fakeProjectBrowser) Port() int { return 6080 }

func (f *fakeProjectContainers) Available() bool { return true }

func (f *fakeProjectContainers) Ensure(context.Context, serviceproject.Meta) error { return nil }

func (f *fakeProjectContainers) Busy(context.Context, string) (bool, error) { return false, nil }

func (f *fakeProjectContainers) Start(context.Context, string) error { return nil }

func (f *fakeProjectContainers) EnsureResources(context.Context, string) error { return nil }

func (f *fakeProjectContainers) SetResourceLimits(_ context.Context, _ string, limits serviceproject.ContainerLimits) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.resourceLimits = limits
	return nil
}

func (f *fakeProjectContainers) currentResourceLimits() serviceproject.ContainerLimits {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.resourceLimits
}

func (f *fakeProjectContainers) Restart(context.Context, string) error { return nil }

func (f *fakeProjectContainers) Stop(context.Context, string) error { return nil }

func (f *fakeProjectContainers) Delete(context.Context, string) error { return nil }

func (f *fakeProjectContainers) State(context.Context, string) (serviceproject.ContainerState, error) {
	return serviceproject.ContainerStateRunning, nil
}

func (f *fakeProjectContainers) Inspect(context.Context, string) (serviceproject.ContainerInspect, error) {
	return serviceproject.ContainerInspect{}, nil
}

func (f *fakeProjectContainers) Repair(context.Context, string) error { return nil }

func (f *fakeProjectContainers) List(context.Context, string) ([]serviceproject.ContainerApp, error) {
	return nil, nil
}

func (f *fakeProjectContainers) ApplyDiff(context.Context, string, map[string]string, []string) error {
	return nil
}

func (f *fakeProjectContainers) ensureBrowser(ctx context.Context, _ string) error {
	f.agentBrowserStartedOnce.Do(func() {
		f.mu.Lock()
		f.agentBrowserStarted = true
		f.mu.Unlock()
		close(f.agentBrowserStartedCh)
	})
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-f.agentBrowserAllowCh:
	}
	f.mu.Lock()
	f.agentBrowserRunning = true
	f.agentBrowserViewRunning = true
	f.mu.Unlock()
	return nil
}

func (f *fakeProjectContainers) stopBrowser(context.Context, string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.agentBrowserRunning = false
	f.agentBrowserViewRunning = false
	f.agentBrowserStoppedFlag = true
	return nil
}

func (f *fakeProjectContainers) stopBrowserView(context.Context, string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.agentBrowserViewRunning = false
	f.agentBrowserViewStoppedFlag = true
	return nil
}

func (f *fakeProjectContainers) browserStatus(context.Context, string) (serviceproject.AgentBrowserInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	info := serviceproject.AgentBrowserInfo{
		Status: serviceproject.AgentBrowserStatusStopped,
		Core:   "off",
		View:   "off",
	}
	if f.agentBrowserRunning {
		info.Status = serviceproject.AgentBrowserStatusCoreReady
		info.Core = "ready"
	}
	if f.agentBrowserViewRunning {
		info.Status = serviceproject.AgentBrowserStatusReady
		info.View = "ready"
	}
	return info, nil
}

func (f *fakeProjectContainers) waitForAgentBrowserStart(t *testing.T) {
	t.Helper()
	select {
	case <-f.agentBrowserStartedCh:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for container Agent Browser start")
	}
}

func (f *fakeProjectContainers) completeAgentBrowserStart() {
	f.agentBrowserAllowOnce.Do(func() {
		close(f.agentBrowserAllowCh)
	})
}

func (f *fakeProjectContainers) agentBrowserStopped() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.agentBrowserStoppedFlag
}

func (f *fakeProjectContainers) agentBrowserViewStopped() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.agentBrowserViewStoppedFlag
}

func waitForAgentBrowserReady(t *testing.T, handler *ProjectHandler, project serviceproject.Meta) agentBrowserResponse {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for {
		req := httptest.NewRequest(http.MethodGet, "/api/projects/"+string(project.ID)+"/agent-browser", nil)
		req.Host = "remote.futrx.com"
		req.Header.Set("X-Forwarded-Proto", "https")
		rec := httptest.NewRecorder()
		handler.HandleResource(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET ready status = %d body=%s", rec.Code, rec.Body.String())
		}
		var status agentBrowserResponse
		if err := json.NewDecoder(rec.Body).Decode(&status); err != nil {
			t.Fatal(err)
		}
		if status.Status == serviceproject.AgentBrowserStatusReady {
			return status
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for ready status, last response = %#v", status)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
