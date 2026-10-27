package cli

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/agent/provisioning"
	serviceprofiles "github.com/futrx-com/remote.futrx.com/internal/service/container/profiles"
)

type runtimeResponse struct {
	out string
	err error
}

type recordingRuntime struct {
	available      bool
	calls          []string
	versions       []runtimeResponse
	commandExists  map[string]bool
	installRunning bool
	installOut     string
	installErr     error
	repairOut      string
	repairErr      error
	installHook    func()
	versionTimeout []time.Duration
	installTimeout time.Duration
	repairTimeout  time.Duration
}

func (r *recordingRuntime) Available() bool { return r.available }

func (r *recordingRuntime) Version(ctx context.Context, containerName, binary string, arguments ...string) (string, error) {
	call := "version " + containerName + " " + binary
	if len(arguments) > 0 {
		call += " " + strings.Join(arguments, " ")
	}
	r.calls = append(r.calls, call)
	r.versionTimeout = append(r.versionTimeout, remainingTimeout(ctx))
	if len(r.versions) == 0 {
		return "", nil
	}
	response := r.versions[0]
	r.versions = r.versions[1:]
	return response.out, response.err
}

func (r *recordingRuntime) CommandExists(_ context.Context, containerName, command string) bool {
	r.calls = append(r.calls, "exists "+containerName+" "+command)
	return r.commandExists[command]
}

func (r *recordingRuntime) InstallRunning(_ context.Context, containerName, packageName string) bool {
	r.calls = append(r.calls, "install running "+containerName+" "+packageName)
	return r.installRunning
}

func (r *recordingRuntime) InstallNPM(ctx context.Context, containerName, npmPackage string) (string, error) {
	r.calls = append(r.calls, "install npm "+containerName+" "+npmPackage)
	r.installTimeout = remainingTimeout(ctx)
	if r.installHook != nil {
		r.installHook()
	}
	return r.installOut, r.installErr
}

func (r *recordingRuntime) Repair(ctx context.Context, containerName, script string) (string, error) {
	r.calls = append(r.calls, "repair "+containerName+" "+script)
	r.repairTimeout = remainingTimeout(ctx)
	return r.repairOut, r.repairErr
}

func remainingTimeout(ctx context.Context) time.Duration {
	deadline, ok := ctx.Deadline()
	if !ok {
		return 0
	}
	return time.Until(deadline)
}

func TestEnsureReturnsImmediatelyWhenUnversionedCLIExists(t *testing.T) {
	runtime := &recordingRuntime{
		available:     true,
		commandExists: map[string]bool{"agent": true},
	}
	spec := provisioning.CLISpec{Binary: "agent", PackageName: "@vendor/agent"}

	if err := NewProvisioner(runtime, serviceprofiles.NewCatalog(nil), nil).
		Ensure(context.Background(), "c1", spec); err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	wantCalls := []string{"exists c1 agent"}
	if !reflect.DeepEqual(runtime.calls, wantCalls) {
		t.Fatalf("calls = %#v, want %#v", runtime.calls, wantCalls)
	}
}

func TestEnsureReportsUnavailableRuntimeWithoutQueries(t *testing.T) {
	runtime := &recordingRuntime{}

	err := NewProvisioner(runtime, serviceprofiles.NewCatalog(nil), nil).
		Ensure(context.Background(), "c1", provisioning.CLISpec{})

	if err == nil || err.Error() != "lxc not available" {
		t.Fatalf("Ensure error = %v", err)
	}
	if len(runtime.calls) != 0 {
		t.Fatalf("calls = %v, want none", runtime.calls)
	}
}

func TestEnsureKeepsNPMInstallSequenceAndTimeout(t *testing.T) {
	runtime := &recordingRuntime{
		available: true,
		versions: []runtimeResponse{
			{out: "agent 1.2.2"},
			{out: "agent 1.2.3"},
		},
		commandExists: map[string]bool{"npm": true},
	}
	spec := provisioning.CLISpec{
		Name:               "agent",
		Binary:             "agent",
		VersionArgs:        []string{"version"},
		PackageName:        "@vendor/agent",
		Version:            "1.2.3",
		CheckVersion:       true,
		VerifyAfterInstall: true,
		InstallTimeout:     5 * time.Minute,
	}
	provisioner := NewProvisioner(runtime, serviceprofiles.NewCatalog(nil), func([]provisioning.Profile) (string, error) {
		t.Fatal("repair recipe called for npm install")
		return "", nil
	})

	if err := provisioner.Ensure(context.Background(), "c1", spec); err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	wantCalls := []string{
		"version c1 agent version",
		"install running c1 @vendor/agent",
		"exists c1 npm",
		"install npm c1 @vendor/agent@1.2.3",
		"version c1 agent version",
	}
	if !reflect.DeepEqual(runtime.calls, wantCalls) {
		t.Fatalf("calls = %#v, want %#v", runtime.calls, wantCalls)
	}
	assertApproximateTimeout(t, runtime.installTimeout, spec.InstallTimeout)
}

func TestEnsureWaitsForRunningInstallBeforeStartingAnother(t *testing.T) {
	runtime := &recordingRuntime{
		available:      true,
		versions:       []runtimeResponse{{out: "agent 1.2.2"}, {out: "agent 1.2.3"}},
		installRunning: true,
	}
	spec := provisioning.CLISpec{
		Binary:         "agent",
		PackageName:    "@vendor/agent",
		Version:        "1.2.3",
		CheckVersion:   true,
		WaitTimeout:    2 * time.Minute,
		InstallTimeout: 5 * time.Minute,
	}

	if err := NewProvisioner(runtime, serviceprofiles.NewCatalog(nil), nil).
		Ensure(context.Background(), "c1", spec); err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	wantCalls := []string{
		"version c1 agent",
		"install running c1 @vendor/agent",
		"version c1 agent",
	}
	if !reflect.DeepEqual(runtime.calls, wantCalls) {
		t.Fatalf("calls = %#v, want %#v", runtime.calls, wantCalls)
	}
	assertApproximateTimeout(t, runtime.versionTimeout[1], spec.WaitTimeout)
}

func TestEnsureUsesRepairRecipeWhenNPMIsMissing(t *testing.T) {
	profiles := []provisioning.Profile{{ID: "agent"}}
	runtime := &recordingRuntime{
		available:     true,
		versions:      []runtimeResponse{{out: "agent 1.2.2"}},
		commandExists: map[string]bool{"npm": false},
	}
	spec := provisioning.CLISpec{
		Name:           "agent",
		Binary:         "agent",
		PackageName:    "@vendor/agent",
		Version:        "1.2.3",
		CheckVersion:   true,
		InstallTimeout: 5 * time.Minute,
	}
	recipeCalls := 0
	provisioner := NewProvisioner(runtime, serviceprofiles.NewCatalog(profiles), func(got []provisioning.Profile) (string, error) {
		recipeCalls++
		if !reflect.DeepEqual(got, profiles) {
			t.Fatalf("recipe profiles = %#v, want %#v", got, profiles)
		}
		return "repair-script", nil
	})

	if err := provisioner.Ensure(context.Background(), "c1", spec); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if recipeCalls != 1 {
		t.Fatalf("recipe calls = %d, want 1", recipeCalls)
	}
	wantCalls := []string{
		"version c1 agent",
		"install running c1 @vendor/agent",
		"exists c1 npm",
		"repair c1 repair-script",
	}
	if !reflect.DeepEqual(runtime.calls, wantCalls) {
		t.Fatalf("calls = %#v, want %#v", runtime.calls, wantCalls)
	}
	assertApproximateTimeout(t, runtime.repairTimeout, spec.InstallTimeout)
}

func TestEnsureImageRepairModeBypassesNPMProbe(t *testing.T) {
	runtime := &recordingRuntime{
		available: true,
		versions:  []runtimeResponse{{out: "agent 1.2.2"}},
	}
	spec := provisioning.CLISpec{
		Binary:         "agent",
		PackageName:    "@vendor/agent",
		Version:        "1.2.3",
		CheckVersion:   true,
		InstallMode:    provisioning.InstallWithImageRepair,
		InstallTimeout: 8 * time.Minute,
	}
	provisioner := NewProvisioner(runtime, serviceprofiles.NewCatalog(nil), func([]provisioning.Profile) (string, error) {
		return "repair-script", nil
	})

	if err := provisioner.Ensure(context.Background(), "c1", spec); err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	wantCalls := []string{
		"version c1 agent",
		"install running c1 @vendor/agent",
		"repair c1 repair-script",
	}
	if !reflect.DeepEqual(runtime.calls, wantCalls) {
		t.Fatalf("calls = %#v, want %#v", runtime.calls, wantCalls)
	}
	assertApproximateTimeout(t, runtime.repairTimeout, spec.InstallTimeout)
}

func TestEnsurePreservesRepairPreparationError(t *testing.T) {
	recipeErr := errors.New("recipe failed")
	runtime := &recordingRuntime{
		available:     true,
		versions:      []runtimeResponse{{out: "agent 1.2.2"}},
		commandExists: map[string]bool{"npm": false},
	}
	spec := provisioning.CLISpec{
		Binary:         "agent",
		PackageName:    "@vendor/agent",
		Version:        "1.2.3",
		CheckVersion:   true,
		InstallTimeout: time.Minute,
	}

	err := NewProvisioner(runtime, serviceprofiles.NewCatalog(nil), func([]provisioning.Profile) (string, error) {
		return "", recipeErr
	}).Ensure(context.Background(), "c1", spec)

	if err == nil || err.Error() != "prepare agent CLI repair: recipe failed" {
		t.Fatalf("Ensure error = %v", err)
	}
	if !errors.Is(err, recipeErr) {
		t.Fatalf("Ensure error = %v, want wrapped recipe error", err)
	}
}

func TestEnsureFailedInstallWaitsForConcurrentSuccess(t *testing.T) {
	installErr := errors.New("npm failed")
	runtime := &recordingRuntime{
		available:      true,
		versions:       []runtimeResponse{{out: "agent 1.2.2"}, {out: "agent 1.2.3"}},
		commandExists:  map[string]bool{"npm": true},
		installOut:     "failed",
		installErr:     installErr,
		installRunning: false,
	}
	spec := provisioning.CLISpec{
		Binary:         "agent",
		PackageName:    "@vendor/agent",
		Version:        "1.2.3",
		CheckVersion:   true,
		InstallTimeout: 5 * time.Minute,
	}

	if err := NewProvisioner(runtime, serviceprofiles.NewCatalog(nil), nil).
		Ensure(context.Background(), "c1", spec); err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	wantCalls := []string{
		"version c1 agent",
		"install running c1 @vendor/agent",
		"exists c1 npm",
		"install npm c1 @vendor/agent@1.2.3",
		"version c1 agent",
	}
	if !reflect.DeepEqual(runtime.calls, wantCalls) {
		t.Fatalf("calls = %#v, want %#v", runtime.calls, wantCalls)
	}
	assertApproximateTimeout(t, runtime.versionTimeout[1], failedInstallWaitTimeout)
}

func TestEnsurePreservesInstallFailureTextAndCause(t *testing.T) {
	installErr := errors.New("npm failed")
	ctx, cancel := context.WithCancel(context.Background())
	runtime := &recordingRuntime{
		available:     true,
		versions:      []runtimeResponse{{out: "agent 1.2.2"}, {out: "agent 1.2.2"}},
		commandExists: map[string]bool{"npm": true},
		installOut:    "npm output",
		installErr:    installErr,
		installHook:   cancel,
	}
	spec := provisioning.CLISpec{
		Name:           "agent",
		Binary:         "agent",
		PackageName:    "@vendor/agent",
		Version:        "1.2.3",
		ReportVersion:  true,
		CheckVersion:   true,
		InstallTimeout: 5 * time.Minute,
	}

	err := NewProvisioner(runtime, serviceprofiles.NewCatalog(nil), nil).Ensure(ctx, "c1", spec)

	want := "install agent 1.2.3 in c1: npm failed; output: npm output"
	if err == nil || err.Error() != want {
		t.Fatalf("Ensure error = %v, want %q", err, want)
	}
	if !errors.Is(err, installErr) {
		t.Fatalf("Ensure error = %v, want wrapped install error", err)
	}
}

func TestCLIInstallLabelReportsVersionOnlyWhenRequested(t *testing.T) {
	spec := provisioning.CLISpec{Name: "agent", Version: "1.2.3"}
	if got := cliInstallLabel(spec); got != "agent" {
		t.Fatalf("label without version = %q", got)
	}
	spec.ReportVersion = true
	if got := cliInstallLabel(spec); got != "agent 1.2.3" {
		t.Fatalf("label with version = %q", got)
	}
}

func TestSemanticVersionAtLeast(t *testing.T) {
	tests := []struct {
		name    string
		actual  string
		minimum string
		want    bool
	}{
		{name: "output at pin", actual: "agent-cli 0.144.1", minimum: "0.144.1", want: true},
		{name: "output above pin", actual: "2.1.207 (Agent CLI)", minimum: "2.1.206", want: true},
		{name: "older patch", actual: "agent-cli 0.144.0", minimum: "0.144.1", want: false},
		{name: "same-core prerelease", actual: "agent-cli 0.144.1-alpha.2", minimum: "0.144.1", want: false},
		{name: "newer prerelease core", actual: "agent-cli 0.145.0-alpha.2", minimum: "0.144.1", want: true},
		{name: "unparseable", actual: "agent unknown", minimum: "0.144.1", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := semanticVersionAtLeast(tt.actual, tt.minimum); got != tt.want {
				t.Fatalf("semanticVersionAtLeast(%q, %q) = %v, want %v", tt.actual, tt.minimum, got, tt.want)
			}
		})
	}
}

func assertApproximateTimeout(t *testing.T, got, want time.Duration) {
	t.Helper()
	if got <= want-time.Second || got > want {
		t.Fatalf("timeout = %s, want approximately %s", got, want)
	}
}
