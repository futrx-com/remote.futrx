package image

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/agent/provisioning"
)

type runtimeResponse struct {
	output string
	err    error
}

type recordingRuntime struct {
	available       bool
	events          []string
	launchResponse  runtimeResponse
	scriptResponses []runtimeResponse
	stopResponse    runtimeResponse
	publishResponse runtimeResponse
}

func (r *recordingRuntime) Available() bool {
	r.events = append(r.events, "available")
	return r.available
}

func (r *recordingRuntime) DeleteContainer(_ context.Context, containerName string) (string, error) {
	r.events = append(r.events, "delete "+containerName)
	return "", nil
}

func (r *recordingRuntime) LaunchContainer(_ context.Context, sourceImage, containerName string) (string, error) {
	r.events = append(r.events, "launch "+sourceImage+" "+containerName)
	return r.launchResponse.output, r.launchResponse.err
}

func (r *recordingRuntime) ExecuteScript(_ context.Context, containerName, script string) (string, error) {
	responseIndex := 0
	for _, event := range r.events {
		if strings.HasPrefix(event, "script ") {
			responseIndex++
		}
	}
	r.events = append(r.events, "script "+containerName+" "+script)
	if responseIndex >= len(r.scriptResponses) {
		return "", nil
	}
	response := r.scriptResponses[responseIndex]
	return response.output, response.err
}

func (r *recordingRuntime) StopContainer(_ context.Context, containerName string) (string, error) {
	r.events = append(r.events, "stop "+containerName)
	return r.stopResponse.output, r.stopResponse.err
}

func (r *recordingRuntime) PublishImage(_ context.Context, containerName, alias, description string) (string, error) {
	r.events = append(r.events, "publish "+containerName+" "+alias+" "+description)
	return r.publishResponse.output, r.publishResponse.err
}

type recordingProfileSource struct {
	profiles  []provisioning.Profile
	snapshots int
}

func (s *recordingProfileSource) Snapshot() []provisioning.Profile {
	s.snapshots++
	return append([]provisioning.Profile(nil), s.profiles...)
}

func configuredProfiles() []provisioning.Profile {
	return []provisioning.Profile{{
		ID: "alpha",
		CLI: provisioning.CLISpec{
			ImageLabel:  "alpha-cli",
			Binary:      "alpha",
			PackageName: "@example/alpha",
			Version:     "1.2.3",
		},
	}}
}

func TestBuildPreservesImageWorkflowOrder(t *testing.T) {
	runtime := &recordingRuntime{available: true}
	profiles := &recordingProfileSource{profiles: configuredProfiles()}
	builder := NewBuilder(runtime, profiles, "browser-install", []byte("code-server-install"), nil)
	builder.networkWarmup = 0

	if err := builder.Build(context.Background(), ""); err != nil {
		t.Fatalf("Build: %v", err)
	}
	installScript, err := InstallScript(profiles.profiles)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"available",
		"delete " + baseImageBuilderName,
		"launch " + SourceImage + " " + baseImageBuilderName,
		"script " + baseImageBuilderName + " " + ipv4EgressProbe,
		"script " + baseImageBuilderName + " " + installScript,
		"script " + baseImageBuilderName + " browser-install",
		"script " + baseImageBuilderName + " code-server-install",
		"stop " + baseImageBuilderName,
		"publish " + baseImageBuilderName + " " + Alias + " futrx remote dev base: ubuntu 24.04 + node 22 + alpha-cli",
		"delete " + baseImageBuilderName,
	}
	assertEvents(t, runtime.events, want)
	if profiles.snapshots != 1 {
		t.Fatalf("profile snapshots = %d, want 1", profiles.snapshots)
	}
}

// The probe reports which hop failed, and the three hops have disjoint fixes.
// Blaming Docker for all of them is what shipped before: on a host without
// Docker it produced a confident instruction (`iptables -I DOCKER-USER ...`)
// that fails with "No chain/target/match by that name", and no route to the
// real cause.
func TestIPv4EgressHintBranchesOnTheFailingHop(t *testing.T) {
	for _, tc := range []struct {
		name       string
		probeOut   string
		want       []string
		wantAbsent []string
	}{
		{
			name:       "no address is a DHCP failure, not a firewall one",
			probeOut:   egressNoAddress,
			want:       []string{"never received an IPv4 address", "port 53", "BIND"},
			wantAbsent: []string{"DOCKER-USER", "FORWARD"},
		},
		{
			name:       "no route points at the bridge, not the firewall",
			probeOut:   egressNoRoute,
			want:       []string{"no default route", "ipv4.nat"},
			wantAbsent: []string{"DOCKER-USER"},
		},
		{
			name:     "blocked is the forwarding case, where Docker belongs",
			probeOut: egressBlocked,
			want:     []string{"cannot reach the internet", "Docker", "ip_forward"},
		},
		{
			name:       "an unreadable probe result must not guess",
			probeOut:   "some unrelated failure",
			want:       []string{"could not determine which hop failed"},
			wantAbsent: []string{"Docker", "BIND"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			hint := ipv4EgressHint(tc.probeOut)
			for _, want := range tc.want {
				if !strings.Contains(hint, want) {
					t.Errorf("hint does not mention %q:\n%s", want, hint)
				}
			}
			for _, absent := range tc.wantAbsent {
				if strings.Contains(hint, absent) {
					t.Errorf("hint should not mention %q:\n%s", absent, hint)
				}
			}
			// Every branch has to hand the operator somewhere to go next: the
			// container can see which hop failed but not what on the host is
			// responsible for it.
			if !strings.Contains(hint, diagnosePath) {
				t.Errorf("hint does not point at %s:\n%s", diagnosePath, hint)
			}
		})
	}
}

func TestBuildStopsBeforeAnyStageWhenContainerHasNoIPv4Egress(t *testing.T) {
	runtime := &recordingRuntime{
		available: true,
		scriptResponses: []runtimeResponse{
			{output: egressBlocked, err: errors.New("exit 1")}, // IPv4 egress probe
		},
	}
	builder := NewBuilder(
		runtime,
		&recordingProfileSource{profiles: configuredProfiles()},
		"browser-install",
		[]byte("code-server-install"),
		nil,
	)
	builder.networkWarmup = 0

	err := builder.Build(context.Background(), "")
	if err == nil {
		t.Fatal("Build succeeded, want an IPv4 egress failure")
	}
	// The message has to name the cause, not the probe: this failure used to
	// surface as a curl timeout against github.com four stages later. The
	// probe's own output has to reach the hint, or every failure collapses
	// back to one generic message.
	for _, want := range []string{"cannot reach the internet", "Docker", diagnosePath} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q does not mention %q", err, want)
		}
	}
	for _, event := range runtime.events {
		if strings.Contains(event, "browser-install") || strings.Contains(event, "code-server-install") {
			t.Fatalf("build ran install stages despite no IPv4 egress: %q", runtime.events)
		}
	}
	if len(runtime.events) == 0 || runtime.events[len(runtime.events)-1] != "delete "+baseImageBuilderName {
		t.Fatalf("events = %q; the builder container was not cleaned up", runtime.events)
	}
}

func TestBuildPreservesErrorOutputAndDeferredCleanup(t *testing.T) {
	runtime := &recordingRuntime{
		available: true,
		scriptResponses: []runtimeResponse{
			{}, // IPv4 egress probe
			{}, // install script
			{output: strings.Repeat("x", 2001), err: errors.New("exit 1")},
		},
	}
	builder := NewBuilder(
		runtime,
		&recordingProfileSource{profiles: configuredProfiles()},
		"browser-install",
		[]byte("code-server-install"),
		nil,
	)
	builder.networkWarmup = 0

	err := builder.Build(context.Background(), "custom-alias")
	wantErr := "agent browser install script: exit 1; output: ..." + strings.Repeat("x", 2000)
	if err == nil || err.Error() != wantErr {
		t.Fatalf("Build error = %v, want %q", err, wantErr)
	}
	if len(runtime.events) < 2 || runtime.events[len(runtime.events)-1] != "delete "+baseImageBuilderName {
		t.Fatalf("events = %q; deferred cleanup did not run", runtime.events)
	}
}

func TestBuildUnavailablePreservesErrorAndDoesNotMutateRuntime(t *testing.T) {
	runtime := &recordingRuntime{}
	builder := NewBuilder(
		runtime,
		&recordingProfileSource{profiles: configuredProfiles()},
		"browser-install",
		[]byte("code-server-install"),
		nil,
	)

	err := builder.Build(context.Background(), "custom-alias")
	const want = "lxc CLI not found on PATH - install LXD on the host first"
	if err == nil || err.Error() != want {
		t.Fatalf("Build error = %v, want %q", err, want)
	}
	assertEvents(t, runtime.events, []string{"available"})
}

func assertEvents(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("events:\n got: %q\nwant: %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("event %d:\n got: %q\nwant: %q", i, got[i], want[i])
		}
	}
}
