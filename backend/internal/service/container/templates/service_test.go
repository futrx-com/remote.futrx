package templates

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
)

type fakeRuntime struct {
	mu sync.Mutex

	available bool
	files     map[string]bool
	images    map[string]bool
	scriptErr error
	calls     []string
	pushed    map[string]string
	pushModes map[string]string
	probeErr  error
}

func newFakeRuntime() *fakeRuntime {
	return &fakeRuntime{
		available: true,
		files:     map[string]bool{},
		images:    map[string]bool{},
		pushed:    map[string]string{},
		pushModes: map[string]string{},
	}
}

func (f *fakeRuntime) Available() bool { return f.available }

func (f *fakeRuntime) FileExists(_ context.Context, container, path string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, "exists "+container+" "+path)
	if f.probeErr != nil {
		return false, f.probeErr
	}
	return f.files[path], nil
}

func (f *fakeRuntime) EnsureDirectory(_ context.Context, container, path string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, "mkdir "+container+" "+path)
	return nil
}

func (f *fakeRuntime) PushFile(_ context.Context, container string, content []byte, target, mode string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, "push "+container+" "+target)
	f.pushed[target] = string(content)
	f.pushModes[target] = mode
	f.files[target] = true
	return nil
}

func (f *fakeRuntime) RunScript(_ context.Context, container, script string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, "run "+container)
	if f.scriptErr != nil {
		return "boom output", f.scriptErr
	}
	// A real run ends by writing the marker inside the container.
	f.files[MarkerPath] = true
	return "ok", nil
}

func (f *fakeRuntime) ImageExists(_ context.Context, alias string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, "image "+alias)
	return f.images[alias], nil
}

func (f *fakeRuntime) recorded() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.calls))
	copy(out, f.calls)
	return out
}

func (f *fakeRuntime) setFile(path string, present bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.files[path] = present
}

func testCatalog(t *testing.T) *Catalog {
	t.Helper()
	catalog, err := LoadFS(fstest.MapFS{
		"blank/template.json": &fstest.MapFile{
			Data: []byte(`{"name":"blank","title":"Blank","description":"d","icon":"blank"}`),
		},
		"stack/template.json": &fstest.MapFile{
			Data: []byte(`{"name":"stack","title":"Stack","description":"d","icon":"stack",` +
				`"provisionScript":"provision.sh","agentInstructions":"AGENTS.md",` +
				`"seedFiles":[{"source":"README.md","target":"/workspace/README.md"}],` +
				`"prebuiltImage":true}`),
		},
		"stack/provision.sh": &fstest.MapFile{Data: []byte("echo install\n")},
		"stack/AGENTS.md":    &fstest.MapFile{Data: []byte("# stack\n")},
		"stack/README.md":    &fstest.MapFile{Data: []byte("readme\n")},
	})
	if err != nil {
		t.Fatalf("LoadFS() = %v", err)
	}
	return catalog
}

func TestEnsureIsANoOpForTheDefaultTemplate(t *testing.T) {
	runtime := newFakeRuntime()
	service := NewService(testCatalog(t), runtime)

	<-service.Ensure(context.Background(), "project-1", "blank")

	if calls := runtime.recorded(); len(calls) != 0 {
		t.Fatalf("blank template touched the runtime: %v", calls)
	}
	state := service.Status(context.Background(), "project-1", "blank")
	if state.Status != StatusNone {
		t.Fatalf("Status = %q, want %q", state.Status, StatusNone)
	}
}

func TestEnsureProvisionsThenBecomesANoOp(t *testing.T) {
	runtime := newFakeRuntime()
	service := NewService(testCatalog(t), runtime)
	ctx := context.Background()

	<-service.Ensure(ctx, "project-1", "stack")

	if runtime.pushed["/workspace/README.md"] != "readme\n" {
		t.Fatalf("seed not written: %v", runtime.pushed)
	}
	if runtime.pushed[InstructionsPath] != "# stack\n" {
		t.Fatalf("agent instructions not seeded: %v", runtime.pushed)
	}
	if state := service.Status(ctx, "project-1", "stack"); state.Status != StatusDone {
		t.Fatalf("Status = %q, want %q", state.Status, StatusDone)
	}

	// Second convergence: the marker is present, so no script and no seeds.
	before := len(runtime.recorded())
	<-service.Ensure(ctx, "project-1", "stack")
	after := runtime.recorded()
	for _, call := range after[before:] {
		if strings.HasPrefix(call, "run ") || strings.HasPrefix(call, "push ") {
			t.Fatalf("re-run performed work: %v", after[before:])
		}
	}
}

func TestEnsureNeverOverwritesAnExistingSeed(t *testing.T) {
	runtime := newFakeRuntime()
	runtime.setFile("/workspace/README.md", true)
	service := NewService(testCatalog(t), runtime)

	<-service.Ensure(context.Background(), "project-1", "stack")

	if _, written := runtime.pushed["/workspace/README.md"]; written {
		t.Fatal("an existing workspace file was overwritten")
	}
	if runtime.pushed[InstructionsPath] == "" {
		t.Fatal("the absent agent-instructions seed should still be written")
	}
}

func TestEnsureSkipsWhenTheMarkerIsAlreadyPresent(t *testing.T) {
	// A container launched from a pre-built template image already carries
	// the marker, so the slow path must not run.
	runtime := newFakeRuntime()
	runtime.setFile(MarkerPath, true)
	service := NewService(testCatalog(t), runtime)

	<-service.Ensure(context.Background(), "project-1", "stack")

	for _, call := range runtime.recorded() {
		if strings.HasPrefix(call, "run ") || strings.HasPrefix(call, "push ") {
			t.Fatalf("provisioning ran despite the marker: %v", runtime.recorded())
		}
	}
	if state := service.Status(context.Background(), "project-1", "stack"); state.Status != StatusDone {
		t.Fatalf("Status = %q, want %q", state.Status, StatusDone)
	}
}

func TestEnsureRecordsFailure(t *testing.T) {
	runtime := newFakeRuntime()
	runtime.scriptErr = errors.New("exit 1")
	service := NewService(testCatalog(t), runtime)
	ctx := context.Background()

	<-service.Ensure(ctx, "project-1", "stack")

	state := service.Status(ctx, "project-1", "stack")
	if state.Status != StatusFailed {
		t.Fatalf("Status = %q, want %q", state.Status, StatusFailed)
	}
	if !strings.Contains(state.Error, "exit 1") || !strings.Contains(state.Error, "boom output") {
		t.Fatalf("Error = %q, want the cause and the command output", state.Error)
	}
	if state.LogPath != LogPath {
		t.Fatalf("LogPath = %q, want %q", state.LogPath, LogPath)
	}
}

func TestStatusFallsBackToTheContainerMarkers(t *testing.T) {
	catalog := testCatalog(t)
	ctx := context.Background()

	tests := []struct {
		name     string
		marker   bool
		failure  bool
		want     Status
		template string
	}{
		{name: "pending", template: "stack", want: StatusPending},
		{name: "done", template: "stack", marker: true, want: StatusDone},
		{name: "failed", template: "stack", failure: true, want: StatusFailed},
		{name: "none", template: "blank", want: StatusNone},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runtime := newFakeRuntime()
			runtime.setFile(MarkerPath, tt.marker)
			runtime.setFile(FailurePath, tt.failure)
			// A fresh service has no in-memory state, exactly like the first
			// status request after a backend restart.
			service := NewService(catalog, runtime)
			if got := service.Status(ctx, "project-1", tt.template); got.Status != tt.want {
				t.Fatalf("Status = %q, want %q", got.Status, tt.want)
			}
		})
	}
}

func TestStatusReportsPendingWhenTheRuntimeIsUnavailable(t *testing.T) {
	runtime := newFakeRuntime()
	runtime.available = false
	service := NewService(testCatalog(t), runtime)

	state := service.Status(context.Background(), "project-1", "stack")
	if state.Status != StatusPending {
		t.Fatalf("Status = %q, want %q", state.Status, StatusPending)
	}
	if calls := runtime.recorded(); len(calls) != 0 {
		t.Fatalf("an unavailable runtime was called: %v", calls)
	}
}

func TestStatusUsesTheProjectsOwnTemplate(t *testing.T) {
	// Cached state belongs to a container/template pair: a project recreated
	// under a different template must not inherit the old answer.
	runtime := newFakeRuntime()
	service := NewService(testCatalog(t), runtime)
	<-service.Ensure(context.Background(), "project-1", "stack")

	state := service.Status(context.Background(), "project-1", "blank")
	if state.Status != StatusNone || state.Template != "blank" {
		t.Fatalf("Status = %+v, want the blank template's own answer", state)
	}
}

func TestImageForPrefersAPublishedTemplateImage(t *testing.T) {
	tests := []struct {
		name      string
		template  string
		published bool
		want      string
	}{
		{name: "published", template: "stack", published: true, want: "futrx-remote-stack-base"},
		{name: "not published", template: "stack", want: ""},
		{name: "template declares none", template: "blank", published: true, want: ""},
		{name: "unknown template falls back", template: "gone", published: true, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runtime := newFakeRuntime()
			if tt.published {
				runtime.images["futrx-remote-stack-base"] = true
			}
			service := NewService(testCatalog(t), runtime)
			if got := service.ImageFor(context.Background(), tt.template); got != tt.want {
				t.Fatalf("ImageFor(%q) = %q, want %q", tt.template, got, tt.want)
			}
		})
	}
}

func TestListAnnotatesPrebuiltImageAvailability(t *testing.T) {
	runtime := newFakeRuntime()
	runtime.images["futrx-remote-stack-base"] = true
	service := NewService(testCatalog(t), runtime)

	descriptors := service.List(context.Background())
	if len(descriptors) != 2 {
		t.Fatalf("List() = %d entries, want 2", len(descriptors))
	}
	byName := map[string]Descriptor{}
	for _, descriptor := range descriptors {
		byName[descriptor.Name] = descriptor
	}
	if !byName["blank"].Default || byName["blank"].Provisions {
		t.Fatalf("blank descriptor = %+v", byName["blank"])
	}
	if byName["blank"].PrebuiltImage != "" || byName["blank"].PrebuiltImageAvailable {
		t.Fatalf("blank must not advertise a template image: %+v", byName["blank"])
	}
	stack := byName["stack"]
	if stack.PrebuiltImage != "futrx-remote-stack-base" || !stack.PrebuiltImageAvailable {
		t.Fatalf("stack descriptor = %+v", stack)
	}
	if !stack.Provisions || stack.Default {
		t.Fatalf("stack descriptor = %+v", stack)
	}
}

func TestTemplateStatusAdaptsToTheProjectPayload(t *testing.T) {
	runtime := newFakeRuntime()
	service := NewService(testCatalog(t), runtime)
	ctx := context.Background()
	<-service.Ensure(ctx, "project-1", "stack")

	status := service.TemplateStatus(ctx, "project-1", "stack")
	if status.Name != "stack" || status.Title != "Stack" || status.Status != string(StatusDone) {
		t.Fatalf("TemplateStatus() = %+v", status)
	}
	if status.StartedAt == 0 || status.FinishedAt == 0 {
		t.Fatalf("TemplateStatus() should carry timestamps: %+v", status)
	}
}

func TestForgetTemplateStateDropsCachedState(t *testing.T) {
	runtime := newFakeRuntime()
	service := NewService(testCatalog(t), runtime)
	ctx := context.Background()
	<-service.Ensure(ctx, "project-1", "stack")

	service.ForgetTemplateState("project-1")
	if _, ok := service.state("project-1"); ok {
		t.Fatal("ForgetTemplateState() left cached state behind")
	}
}
