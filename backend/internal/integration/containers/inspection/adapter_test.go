package inspection

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/agent/provisioning"
	"github.com/futrx-com/remote.futrx.com/internal/integration/containers/assets"
	serviceprofiles "github.com/futrx-com/remote.futrx.com/internal/service/container/profiles"
	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
)

var testAgentInstructions = provisioning.InstructionsTemplate("remote.futrx.com")

func TestAdapterLXDProbesPopulateProviderNeutralSnapshot(t *testing.T) {
	runner := &inspectionRecordingRunner{responses: map[string]inspectionResponse{
		"query /1.0/instances/c1": {output: `{
			"architecture":"x86_64",
			"type":"container",
			"created_at":"2026-01-02T03:04:05Z",
			"last_used_at":"2026-01-03T04:05:06Z",
			"config":{
				"boot.autostart":"true",
				"image.alias":"futrx-base",
				"limits.cpu":"4"
			},
			"expanded_config":{
				"limits.cpu":"2",
				"limits.memory":"8GiB"
			},
			"expanded_devices":{"root":{"type":"disk","path":"/","size":"40GiB"}},
			"devices":{"workspace":{"source":"/host/workspace","path":"/workspace"}}
		}`},
		"query /1.0/instances/c1/state": {output: `{
			"pid":42,
			"processes":7,
			"cpu":{"usage":3000000000},
			"disk":{"root":{"usage":100,"total":200}},
			"memory":{"usage":10,"usage_peak":20,"total":30,"swap_usage":4},
			"network":{"eth0":{
				"hwaddr":"00:11:22:33:44:55",
				"host_name":"veth0",
				"mtu":1500,
				"state":"up",
				"type":"broadcast",
				"counters":{"bytes_received":12,"bytes_sent":13},
				"addresses":[{"address":"10.0.0.2","netmask":"24"}]
			}}
		}`},
	}}
	adapter := NewAdapter(runner, serviceprofiles.NewCatalog(nil), testAgentInstructions)
	out := serviceproject.ContainerInspect{Name: "c1"}

	adapter.InspectConfiguration(context.Background(), "c1", &out)
	adapter.InspectRuntime(context.Background(), "c1", &out)

	if out.Architecture != "x86_64" || out.Type != "container" || !out.BootAutostart || out.Image != "futrx-base" {
		t.Fatalf("configuration fields = %#v", out)
	}
	if out.Limits == nil || *out.Limits != (serviceproject.ContainerLimits{CPU: "4", Memory: "8GiB", Disk: "40GiB"}) {
		t.Fatalf("limits = %#v", out.Limits)
	}
	if out.Workspace == nil || out.Workspace.HostSource != "/host/workspace" || out.Workspace.ContainerPath != "/workspace" {
		t.Fatalf("workspace = %#v", out.Workspace)
	}
	if out.PID != 42 || out.Resources == nil || out.Resources.Processes != 7 || out.Resources.CPUUsageSeconds != 3 || out.Resources.DiskUsageBytes != 100 {
		t.Fatalf("runtime fields = %#v", out)
	}
	if len(out.Network) != 1 || out.Network[0].Name != "eth0" || !reflect.DeepEqual(out.Network[0].Addresses, []string{"10.0.0.2/24"}) {
		t.Fatalf("network = %#v", out.Network)
	}
}

func TestAdapterLXDProbeFailuresLeaveExistingSnapshotUntouched(t *testing.T) {
	runner := &inspectionRecordingRunner{fallback: inspectionResponse{err: errors.New("query failed")}}
	adapter := NewAdapter(runner, serviceprofiles.NewCatalog(nil), testAgentInstructions)
	want := serviceproject.ContainerInspect{Name: "c1", State: serviceproject.ContainerStateRunning}
	got := want

	adapter.InspectConfiguration(context.Background(), "c1", &got)
	adapter.InspectRuntime(context.Background(), "c1", &got)

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("snapshot = %#v, want %#v", got, want)
	}
}

func TestAdapterGuestAndAgentProbesTranslateCommandOutput(t *testing.T) {
	instructionHash := assets.Hash(testAgentInstructions)
	runner := &inspectionRecordingRunner{respond: func(args []string) inspectionResponse {
		joined := strings.Join(args, " ")
		switch joined {
		case "exec c1 -- which claude":
			return inspectionResponse{}
		case "exec c1 -- claude version --short":
			return inspectionResponse{output: "claude 1.2.3\n"}
		case "exec c1 -- test -f /workspace/CLAUDE.md":
			return inspectionResponse{}
		case "exec c1 -- cat /workspace/.claude.hash":
			return inspectionResponse{output: instructionHash + "\n"}
		}
		if len(args) >= 5 && args[0] == "exec" && args[3] == "sh" && args[4] == "-c" {
			return inspectionResponse{output: `=== OS_RELEASE ===
PRETTY_NAME="Ubuntu 24.04"
=== KERNEL ===
6.8.0
=== HOSTNAME ===
c1
=== NPROC ===
4
=== UPTIME ===
12.75 1.00
=== DF ===
Filesystem 1B-blocks Used Available Capacity MountedOn
/dev/root 1000 400 600 40% /
=== END ===
`}
		}
		return inspectionResponse{err: errors.New("unexpected command: " + joined)}
	}}
	profiles := serviceprofiles.NewCatalog([]provisioning.Profile{{
		ID:  "claude",
		CLI: provisioning.CLISpec{Name: "Claude Code", Binary: "claude", VersionArgs: []string{"version", "--short"}},
		Instructions: &provisioning.InstructionTarget{
			Path:     "/workspace/CLAUDE.md",
			HashPath: "/workspace/.claude.hash",
		},
	}})
	adapter := NewAdapter(runner, profiles, testAgentInstructions)

	osInfo, disks := adapter.InspectGuest(context.Background(), "c1")
	agents := adapter.InspectAgents(context.Background(), "c1")

	if osInfo == nil || *osInfo != (serviceproject.OSInfo{
		PrettyName: "Ubuntu 24.04",
		Kernel:     "6.8.0",
		Hostname:   "c1",
		CPUCount:   4,
		UptimeSec:  12,
	}) {
		t.Fatalf("OS = %#v", osInfo)
	}
	if want := []serviceproject.DiskUsage{{
		MountPath:  "/",
		Filesystem: "/dev/root",
		TotalBytes: 1000,
		UsedBytes:  400,
		AvailBytes: 600,
	}}; !reflect.DeepEqual(disks, want) {
		t.Fatalf("disks = %#v, want %#v", disks, want)
	}
	if want := []serviceproject.AgentContainerStatus{{
		ID:                    "claude",
		Label:                 "Claude Code",
		Installed:             true,
		Version:               "claude 1.2.3",
		InstructionsPath:      "/workspace/CLAUDE.md",
		InstructionsInstalled: true,
		InstructionsInSync:    true,
	}}; !reflect.DeepEqual(agents, want) {
		t.Fatalf("agents = %#v, want %#v", agents, want)
	}
}

func TestAdapterCredentialProbeGatesGuestStatsByState(t *testing.T) {
	hostPath := filepath.Join(t.TempDir(), "credentials.json")
	if err := os.WriteFile(hostPath, []byte("{}"), 0o600); err != nil {
		t.Fatalf("write host credential: %v", err)
	}
	hostTime := time.Unix(200, 0)
	if err := os.Chtimes(hostPath, hostTime, hostTime); err != nil {
		t.Fatalf("set host credential time: %v", err)
	}
	runner := &inspectionRecordingRunner{responses: map[string]inspectionResponse{
		"exec c1 -- stat -c %Y /root/.credentials.json": {output: "100\n"},
	}}
	profiles := serviceprofiles.NewCatalog([]provisioning.Profile{{
		Credentials: provisioning.CredentialSpec{
			Name: "claude",
			Files: []provisioning.CredentialFile{{
				HostPath:      hostPath,
				ContainerPath: "/root/.credentials.json",
			}},
		},
	}})
	adapter := NewAdapter(runner, profiles, testAgentInstructions)

	stopped := adapter.InspectCredentials(context.Background(), "c1", serviceproject.ContainerStateStopped)
	if len(runner.calls) != 0 {
		t.Fatalf("stopped probe executed guest commands: %v", runner.calls)
	}
	if len(stopped) != 1 || len(stopped[0].Files) != 1 || !stopped[0].Files[0].HostExists || stopped[0].Files[0].ContainerExists {
		t.Fatalf("stopped credentials = %#v", stopped)
	}

	running := adapter.InspectCredentials(context.Background(), "c1", serviceproject.ContainerStateRunning)
	file := running[0].Files[0]
	if !file.HostExists || !file.ContainerExists || file.HostMTime != 200 || file.ContainerMTime != 100 || !file.HostNewer || file.ContainerNewer {
		t.Fatalf("running credentials = %#v", running)
	}
}

func TestAdapterCredentialProbeIncludesDynamicDirectoryFiles(t *testing.T) {
	hostDirectory := t.TempDir()
	hostPath := filepath.Join(hostDirectory, "host.json")
	if err := os.WriteFile(hostPath, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	hostTime := time.Unix(100, 0)
	if err := os.Chtimes(hostPath, hostTime, hostTime); err != nil {
		t.Fatal(err)
	}
	containerDirectory := "/root/.future/credentials"
	runner := &inspectionRecordingRunner{responses: map[string]inspectionResponse{
		"exec c1 -- find " + containerDirectory + " -mindepth 1 -maxdepth 1 -type f -printf %f\\n": {
			output: "container.json\nhost.json\n../unsafe\n",
		},
		"exec c1 -- stat -c %Y " + containerDirectory + "/container.json": {output: "200\n"},
		"exec c1 -- stat -c %Y " + containerDirectory + "/host.json":      {output: "90\n"},
	}}
	profiles := serviceprofiles.NewCatalog([]provisioning.Profile{{
		ID: "future-agent",
		Credentials: provisioning.CredentialSpec{
			Name: "future-agent",
			Directory: &provisioning.CredentialDirectory{
				HostPath: hostDirectory, ContainerPath: containerDirectory,
			},
		},
	}})
	adapter := NewAdapter(runner, profiles, testAgentInstructions)

	running := adapter.InspectCredentials(context.Background(), "c1", serviceproject.ContainerStateRunning)
	if len(running) != 1 || len(running[0].Files) != 2 {
		t.Fatalf("running directory credentials = %#v", running)
	}
	if got := running[0].Files[0]; filepath.Base(got.HostPath) != "container.json" || !got.ContainerExists || !got.ContainerNewer {
		t.Fatalf("container-only credential = %#v", got)
	}
	if got := running[0].Files[1]; filepath.Base(got.HostPath) != "host.json" || !got.HostExists || !got.ContainerExists || !got.HostNewer {
		t.Fatalf("shared credential = %#v", got)
	}

	runner.calls = nil
	stopped := adapter.InspectCredentials(context.Background(), "c1", serviceproject.ContainerStateStopped)
	if len(runner.calls) != 0 {
		t.Fatalf("stopped directory probe executed guest commands: %v", runner.calls)
	}
	if len(stopped) != 1 || len(stopped[0].Files) != 1 || filepath.Base(stopped[0].Files[0].HostPath) != "host.json" {
		t.Fatalf("stopped directory credentials = %#v", stopped)
	}
}

func TestAdapterCredentialProbeEncodesAnEmptyDirectoryAsAnArray(t *testing.T) {
	hostDirectory := t.TempDir()
	containerDirectory := "/root/.future/credentials"
	runner := &inspectionRecordingRunner{responses: map[string]inspectionResponse{
		"exec c1 -- find " + containerDirectory + " -mindepth 1 -maxdepth 1 -type f -printf %f\\n": {},
	}}
	profiles := serviceprofiles.NewCatalog([]provisioning.Profile{{
		ID: "future-agent",
		Credentials: provisioning.CredentialSpec{
			Name: "future-agent",
			Directory: &provisioning.CredentialDirectory{
				HostPath: hostDirectory, ContainerPath: containerDirectory,
			},
		},
	}})
	adapter := NewAdapter(runner, profiles, testAgentInstructions)

	bundles := adapter.InspectCredentials(context.Background(), "c1", serviceproject.ContainerStateRunning)
	if len(bundles) != 1 || bundles[0].Files == nil || len(bundles[0].Files) != 0 {
		t.Fatalf("empty directory credentials = %#v", bundles)
	}
	encoded, err := json.Marshal(bundles)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"files":[]`) {
		t.Fatalf("empty directory credentials JSON = %s", encoded)
	}
}

type inspectionResponse struct {
	output string
	err    error
}

type inspectionRecordingRunner struct {
	responses map[string]inspectionResponse
	fallback  inspectionResponse
	respond   func(args []string) inspectionResponse
	calls     []string
}

func (*inspectionRecordingRunner) Available() bool { return true }

func (r *inspectionRecordingRunner) Run(_ context.Context, args ...string) (string, error) {
	joined := strings.Join(args, " ")
	r.calls = append(r.calls, joined)
	if r.respond != nil {
		response := r.respond(args)
		return response.output, response.err
	}
	if response, ok := r.responses[joined]; ok {
		return response.output, response.err
	}
	return r.fallback.output, r.fallback.err
}

func (*inspectionRecordingRunner) RunStdin(context.Context, io.Reader, ...string) (string, error) {
	return "", nil
}
