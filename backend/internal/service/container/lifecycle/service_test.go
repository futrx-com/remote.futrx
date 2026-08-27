package lifecycle

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/agent/provisioning"
	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
)

type testDisk struct{ source, path string }

type recordingRuntime struct {
	events    *[]string
	available bool
	state     serviceproject.ContainerState
	devices   map[string]testDisk
	busy      bool
	initErr   error
	attachErr error
	deleteErr error
	mountMiss int
}

func (r *recordingRuntime) Available() bool {
	*r.events = append(*r.events, "runtime available")
	return r.available
}
func (r *recordingRuntime) Init(_ context.Context, image, container string) error {
	*r.events = append(*r.events, "runtime init "+image+" "+container)
	if r.initErr == nil {
		r.state = serviceproject.ContainerStateStopped
	}
	return r.initErr
}
func (r *recordingRuntime) Disk(_ context.Context, container, name string) (string, string, bool, error) {
	*r.events = append(*r.events, "runtime disk "+container+" "+name)
	disk, ok := r.devices[name]
	return disk.source, disk.path, ok, nil
}
func (r *recordingRuntime) AttachDisk(_ context.Context, container, name, source, path string, _ bool) error {
	*r.events = append(*r.events, "runtime attach "+container+" "+name+" "+source+" "+path)
	if r.attachErr == nil {
		if r.devices == nil {
			r.devices = make(map[string]testDisk)
		}
		r.devices[name] = testDisk{source: source, path: path}
	}
	return r.attachErr
}
func (r *recordingRuntime) RemoveDevice(_ context.Context, container, name string) error {
	*r.events = append(*r.events, "runtime remove "+container+" "+name)
	delete(r.devices, name)
	return nil
}
func (r *recordingRuntime) PullDirectory(_ context.Context, container, source, target string) (bool, error) {
	*r.events = append(*r.events, "runtime pull "+container+" "+source)
	dir := filepath.Join(target, filepath.Base(source))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return false, err
	}
	if err := os.WriteFile(filepath.Join(dir, "legacy-session"), []byte("state"), 0o600); err != nil {
		return false, err
	}
	return true, nil
}
func (r *recordingRuntime) Mounted(_ context.Context, container, path string) (bool, error) {
	*r.events = append(*r.events, "runtime mounted "+container+" "+path)
	if r.mountMiss > 0 {
		r.mountMiss--
		return false, nil
	}
	for _, disk := range r.devices {
		if disk.path == path {
			return true, nil
		}
	}
	return false, nil
}
func (r *recordingRuntime) Busy(context.Context, string) (bool, error) { return r.busy, nil }
func (r *recordingRuntime) EnsureBootAutostart(_ context.Context, container string) error {
	*r.events = append(*r.events, "runtime autostart "+container)
	return nil
}
func (r *recordingRuntime) Start(_ context.Context, container string) error {
	*r.events = append(*r.events, "runtime start "+container)
	r.state = serviceproject.ContainerStateRunning
	return nil
}
func (r *recordingRuntime) Stop(_ context.Context, container string) error {
	*r.events = append(*r.events, "runtime stop "+container)
	r.state = serviceproject.ContainerStateStopped
	return nil
}
func (r *recordingRuntime) Restart(_ context.Context, container string) error {
	*r.events = append(*r.events, "runtime restart "+container)
	r.state = serviceproject.ContainerStateRunning
	return nil
}
func (r *recordingRuntime) Delete(_ context.Context, container string) error {
	*r.events = append(*r.events, "runtime delete "+container)
	if r.deleteErr == nil {
		r.state = serviceproject.ContainerStateMissing
	}
	return r.deleteErr
}
func (r *recordingRuntime) State(_ context.Context, container string) (serviceproject.ContainerState, error) {
	*r.events = append(*r.events, "runtime state "+container)
	return r.state, nil
}

type recordingWorkspace struct{ events *[]string }

func (w recordingWorkspace) Prepare(path string) error {
	*w.events = append(*w.events, "prepare "+path)
	return os.MkdirAll(path, 0o755)
}

type recordingResources struct{ events *[]string }

func (r recordingResources) Ensure(_ context.Context, container string) error {
	*r.events = append(*r.events, "resources "+container)
	return nil
}
func (r recordingResources) SetLimits(_ context.Context, container, cpu, memory, disk string) error {
	*r.events = append(*r.events, "limits "+container+" "+cpu+" "+memory+" "+disk)
	return nil
}

type recordingProvisioner struct{ events *[]string }

func (p recordingProvisioner) Provision(_ context.Context, container, name string) {
	*p.events = append(*p.events, "provision "+container+" "+name)
}

type testProfileSource struct{}

func (testProfileSource) Snapshot() []provisioning.Profile {
	return []provisioning.Profile{
		{ID: "claude", PersistentState: []provisioning.PersistentDirectory{{Device: "claude-home", HostDirectory: "claude", ContainerPath: "/root/.claude"}}},
		{ID: "codex", PersistentState: []provisioning.PersistentDirectory{{Device: "codex-home", HostDirectory: "codex", ContainerPath: "/root/.codex"}}},
		{ID: "kimi", PersistentState: []provisioning.PersistentDirectory{{Device: "kimi-home", HostDirectory: "kimi", ContainerPath: "/root/.kimi-code"}}},
		{ID: "antigravity", PersistentState: []provisioning.PersistentDirectory{{Device: "antigravity-home", HostDirectory: "antigravity", ContainerPath: "/root/.gemini/antigravity-cli"}}},
	}
}

func testProject(t *testing.T) serviceproject.Meta {
	t.Helper()
	return serviceproject.Meta{
		Name:          "My Project",
		Cwd:           filepath.Join(t.TempDir(), "project", "workspace"),
		ContainerName: "project-1",
	}
}

func newTestService(runtime *recordingRuntime, events *[]string) *Service {
	return NewService(
		runtime,
		"local:remote-base",
		recordingWorkspace{events: events},
		recordingResources{events: events},
		recordingProvisioner{events: events},
		testProfileSource{},
	)
}

func expectedDisks(project serviceproject.Meta) map[string]testDisk {
	mounts, _, _ := persistentMounts(project, testProfileSource{}.Snapshot())
	out := make(map[string]testDisk, len(mounts))
	for _, mount := range mounts {
		out[mount.device] = testDisk{source: mount.hostPath, path: mount.containerPath}
	}
	return out
}

func TestEnsureCreatesStoppedContainerThenAttachesAndValidatesAllDurableMounts(t *testing.T) {
	var events []string
	runtime := &recordingRuntime{events: &events, available: true, state: serviceproject.ContainerStateMissing}
	project := testProject(t)

	if err := newTestService(runtime, &events).Ensure(context.Background(), project); err != nil {
		t.Fatal(err)
	}
	if runtime.state != serviceproject.ContainerStateRunning {
		t.Fatalf("state = %q, want running", runtime.state)
	}
	if len(runtime.devices) != 5 {
		t.Fatalf("devices = %#v, want five durable mounts", runtime.devices)
	}
	initAt := slices.Index(events, "runtime init local:remote-base project-1")
	startAt := slices.Index(events, "runtime start project-1")
	attachAt := slices.Index(events, "runtime attach project-1 workspace "+project.Cwd+" /workspace")
	if initAt < 0 || attachAt < initAt || startAt < attachAt {
		t.Fatalf("container was not initialized, mounted, then started: %q", events)
	}
	if !slices.Contains(events, "provision project-1 My Project") {
		t.Fatalf("launch provisioning missing: %q", events)
	}
}

func TestPersistentMountsIncludeFutureProfileStateWithoutLifecycleChanges(t *testing.T) {
	project := testProject(t)
	profiles := append(testProfileSource{}.Snapshot(), provisioning.Profile{
		ID: "future-agent",
		PersistentState: []provisioning.PersistentDirectory{{
			Device: "future-home", HostDirectory: "future", ContainerPath: "/root/.future",
		}},
	})
	mounts, _, err := persistentMounts(project, profiles)
	if err != nil {
		t.Fatal(err)
	}
	wantHost := filepath.Join(filepath.Dir(project.Cwd), agentHomeDirName, "future")
	if !slices.Contains(mounts, (durableMount{
		device: "future-home", hostPath: wantHost, containerPath: "/root/.future", migrateExisting: true,
	})) {
		t.Fatalf("future state mount missing: %#v", mounts)
	}
}

func TestPersistentMountsRejectInvalidAndOverlappingProfileState(t *testing.T) {
	project := testProject(t)
	for name, profiles := range map[string][]provisioning.Profile{
		"unsafe host":      {{ID: "future", PersistentState: []provisioning.PersistentDirectory{{Device: "future-home", HostDirectory: "../future", ContainerPath: "/root/.future"}}}},
		"workspace device": {{ID: "future", PersistentState: []provisioning.PersistentDirectory{{Device: "workspace", HostDirectory: "future", ContainerPath: "/root/.future"}}}},
		"overlapping targets": {{ID: "future", PersistentState: []provisioning.PersistentDirectory{
			{Device: "future-home", HostDirectory: "future", ContainerPath: "/root/.future"},
			{Device: "future-cache", HostDirectory: "future-cache", ContainerPath: "/root/.future/cache"},
		}}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, err := persistentMounts(project, profiles); err == nil {
				t.Fatal("persistentMounts accepted invalid state")
			}
		})
	}
}

func TestEnsureRunningHealthyContainerDoesNotRestart(t *testing.T) {
	var events []string
	project := testProject(t)
	runtime := &recordingRuntime{
		events: &events, available: true, state: serviceproject.ContainerStateRunning,
		devices: expectedDisks(project),
	}

	if err := newTestService(runtime, &events).Ensure(context.Background(), project); err != nil {
		t.Fatal(err)
	}
	if slices.Contains(events, "runtime stop project-1") || slices.Contains(events, "runtime start project-1") {
		t.Fatalf("healthy container restarted: %q", events)
	}
}

func TestEnsureRestartsConfiguredButInactiveMountWhenProjectIsIdle(t *testing.T) {
	var events []string
	project := testProject(t)
	runtime := &recordingRuntime{
		events: &events, available: true, state: serviceproject.ContainerStateRunning,
		devices: expectedDisks(project), mountMiss: 1,
	}

	if err := newTestService(runtime, &events).Ensure(context.Background(), project); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(events, "runtime restart project-1") {
		t.Fatalf("inactive mount was not repaired: %q", events)
	}
}

func TestEnsureDoesNotRestartConfiguredButInactiveMountWhileAgentIsBusy(t *testing.T) {
	var events []string
	project := testProject(t)
	runtime := &recordingRuntime{
		events: &events, available: true, state: serviceproject.ContainerStateRunning,
		devices: expectedDisks(project), mountMiss: 1, busy: true,
	}

	err := newTestService(runtime, &events).Ensure(context.Background(), project)
	if !errors.Is(err, ErrContainerBusy) {
		t.Fatalf("error = %v, want busy", err)
	}
	if slices.Contains(events, "runtime restart project-1") {
		t.Fatalf("busy container was restarted: %q", events)
	}
}

func TestEnsureRepairsRunningContainerWithMissingWorkspaceMount(t *testing.T) {
	var events []string
	project := testProject(t)
	devices := expectedDisks(project)
	delete(devices, "workspace")
	runtime := &recordingRuntime{events: &events, available: true, state: serviceproject.ContainerStateRunning, devices: devices}

	if err := newTestService(runtime, &events).Ensure(context.Background(), project); err != nil {
		t.Fatal(err)
	}
	if runtime.devices["workspace"].source != project.Cwd {
		t.Fatalf("workspace device = %#v", runtime.devices["workspace"])
	}
	if !slices.Contains(events, "runtime stop project-1") || !slices.Contains(events, "runtime start project-1") {
		t.Fatalf("repair did not restart container: %q", events)
	}
}

func TestEnsureMigratesLegacyAgentHomeBeforeAttachingPersistentDisk(t *testing.T) {
	var events []string
	project := testProject(t)
	devices := expectedDisks(project)
	delete(devices, "codex-home")
	runtime := &recordingRuntime{events: &events, available: true, state: serviceproject.ContainerStateRunning, devices: devices}

	if err := newTestService(runtime, &events).Ensure(context.Background(), project); err != nil {
		t.Fatal(err)
	}
	codexHome := runtime.devices["codex-home"].source
	if data, err := os.ReadFile(filepath.Join(codexHome, "legacy-session")); err != nil || string(data) != "state" {
		t.Fatalf("migrated session = %q, %v", data, err)
	}
	pullAt := slices.Index(events, "runtime pull project-1 /root/.codex")
	attachAt := slices.Index(events, "runtime attach project-1 codex-home "+codexHome+" /root/.codex")
	if pullAt < 0 || attachAt < pullAt {
		t.Fatalf("agent home was not migrated before attachment: %q", events)
	}
}

func TestEnsureDoesNotMutateLegacyContainerWhileAgentIsBusy(t *testing.T) {
	var events []string
	project := testProject(t)
	runtime := &recordingRuntime{events: &events, available: true, state: serviceproject.ContainerStateRunning, busy: true}

	err := newTestService(runtime, &events).Ensure(context.Background(), project)
	if !errors.Is(err, ErrContainerBusy) {
		t.Fatalf("error = %v, want busy", err)
	}
	if slices.Contains(events, "runtime stop project-1") {
		t.Fatalf("busy container was stopped: %q", events)
	}
}

func TestEnsureRollsBackNewContainerWhenAttachmentFails(t *testing.T) {
	var events []string
	wantErr := errors.New("attach failed")
	runtime := &recordingRuntime{
		events: &events, available: true, state: serviceproject.ContainerStateMissing, attachErr: wantErr,
	}

	err := newTestService(runtime, &events).Ensure(context.Background(), testProject(t))
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
	if runtime.state != serviceproject.ContainerStateMissing {
		t.Fatalf("state = %q, want missing after rollback", runtime.state)
	}
}

func TestEnsureReportsProvisioningAndRollbackFailures(t *testing.T) {
	var events []string
	attachErr := errors.New("attach failed")
	deleteErr := errors.New("delete failed")
	runtime := &recordingRuntime{
		events: &events, available: true, state: serviceproject.ContainerStateMissing,
		attachErr: attachErr, deleteErr: deleteErr,
	}

	err := newTestService(runtime, &events).Ensure(context.Background(), testProject(t))
	if !errors.Is(err, attachErr) || !errors.Is(err, deleteErr) {
		t.Fatalf("error = %v, want joined attachment and rollback failures", err)
	}
}
