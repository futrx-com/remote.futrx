package hostcli

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/agent/provisioning"
)

const testManagedPrefix = "/managed/host-clis"

type fakeRuntime struct {
	exists           map[string]bool
	executablePaths  map[string]string
	versions         map[string]string
	npmInstalls      []npmInstall
	scriptInstalls   []scriptInstall
	installErr       error
	installedOutput  string
	waitForCancel    bool
	installHook      func()
	versionBinaries  []string
	versionCalls     [][]string
	versionTimeout   []time.Duration
	blockVersionCall int
}

type npmInstall struct {
	prefix     string
	npmPackage string
}

type scriptInstall struct {
	script     string
	executable string
}

func (r *fakeRuntime) CommandExists(executablePath string) bool {
	return r.exists[executablePath]
}

func (r *fakeRuntime) ExecutablePath(binary string) string {
	return r.executablePaths[binary]
}

func (r *fakeRuntime) Version(ctx context.Context, binary string, arguments ...string) (string, error) {
	r.versionBinaries = append(r.versionBinaries, binary)
	r.versionCalls = append(r.versionCalls, append([]string(nil), arguments...))
	r.versionTimeout = append(r.versionTimeout, remainingContextTimeout(ctx))
	if r.blockVersionCall == len(r.versionCalls) {
		<-ctx.Done()
		return "", ctx.Err()
	}
	version, ok := r.versions[binary]
	if !ok {
		return "", errors.New("missing binary")
	}
	return version, nil
}

func (r *fakeRuntime) InstallNPM(ctx context.Context, prefix, npmPackage string) (string, error) {
	r.npmInstalls = append(r.npmInstalls, npmInstall{prefix: prefix, npmPackage: npmPackage})
	if r.waitForCancel {
		<-ctx.Done()
		return "", ctx.Err()
	}
	if r.installHook != nil {
		r.installHook()
	}
	if r.installErr == nil {
		executable := filepath.Join(prefix, "bin", "future")
		r.versions[executable] = r.installedOutput
		r.exists[executable] = true
		r.executablePaths["future"] = executable
	}
	return "npm output", r.installErr
}

func remainingContextTimeout(ctx context.Context) time.Duration {
	deadline, ok := ctx.Deadline()
	if !ok {
		return 0
	}
	return time.Until(deadline)
}

func (r *fakeRuntime) InstallScript(_ context.Context, script, executable string) (string, error) {
	r.scriptInstalls = append(r.scriptInstalls, scriptInstall{script: script, executable: executable})
	if r.installErr == nil {
		r.versions[executable] = r.installedOutput
		r.exists[executable] = true
		r.executablePaths["future"] = executable
	}
	return "script output", r.installErr
}

func TestEnsureAllSkipsMatchingPinnedVersion(t *testing.T) {
	runtime := newFakeRuntime("future 1.2.3")
	results, err := newTestInstaller(runtime, time.Second).EnsureAll(context.Background(), []provisioning.Profile{testProfile(provisioning.InstallWithNPM)})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Changed || results[0].DetectedVersion != "1.2.3" {
		t.Fatalf("results = %#v", results)
	}
	if len(runtime.npmInstalls) != 0 {
		t.Fatalf("npm installs = %#v", runtime.npmInstalls)
	}
	if len(runtime.versionCalls) != 1 || !slices.Equal(runtime.versionCalls[0], []string{"version"}) {
		t.Fatalf("version calls = %#v", runtime.versionCalls)
	}
	if len(runtime.versionBinaries) != 1 || runtime.versionBinaries[0] != testManagedExecutable("future") {
		t.Fatalf("version binaries = %#v", runtime.versionBinaries)
	}
}

func TestEnsureAllInstallsNPMModesAtExactPin(t *testing.T) {
	for _, mode := range []provisioning.InstallMode{
		provisioning.InstallWithNPM,
		provisioning.InstallWithImageRepair,
	} {
		t.Run(string(mode), func(t *testing.T) {
			runtime := newFakeRuntime("future 1.2.2")
			runtime.installedOutput = "future version 1.2.3"
			results, err := newTestInstaller(runtime, time.Second).EnsureAll(context.Background(), []provisioning.Profile{testProfile(mode)})
			if err != nil {
				t.Fatal(err)
			}
			wantInstall := npmInstall{prefix: testManagedPrefix, npmPackage: "future-cli@1.2.3"}
			if len(runtime.npmInstalls) != 1 || runtime.npmInstalls[0] != wantInstall {
				t.Fatalf("npm installs = %#v", runtime.npmInstalls)
			}
			if len(results) != 1 || !results[0].Changed || results[0].DetectedVersion != "1.2.3" {
				t.Fatalf("results = %#v", results)
			}
		})
	}
}

func TestEnsureAllRunsPinnedScriptPolicy(t *testing.T) {
	profile := testProfile(provisioning.InstallWithScript)
	profile.CLI.PackageName = ""
	profile.CLI.InstallScript = "install future 1.2.3"
	runtime := newFakeRuntime("")
	runtime.installedOutput = "1.2.3"
	if _, err := newTestInstaller(runtime, time.Second).EnsureAll(context.Background(), []provisioning.Profile{profile}); err != nil {
		t.Fatal(err)
	}
	wantInstall := scriptInstall{script: profile.CLI.InstallScript, executable: testManagedExecutable("future")}
	if len(runtime.scriptInstalls) != 1 || runtime.scriptInstalls[0] != wantInstall {
		t.Fatalf("script installs = %#v", runtime.scriptInstalls)
	}
	if len(runtime.npmInstalls) != 0 {
		t.Fatalf("npm installs = %#v", runtime.npmInstalls)
	}
}

func TestEnsureAllRejectsSuccessfulInstallWithWrongVersion(t *testing.T) {
	runtime := newFakeRuntime("future 1.2.2")
	runtime.installedOutput = "future 1.2.4"
	_, err := newTestInstaller(runtime, time.Second).EnsureAll(context.Background(), []provisioning.Profile{testProfile(provisioning.InstallWithNPM)})
	if err == nil || !strings.Contains(err.Error(), "managed executable") ||
		!strings.Contains(err.Error(), `detected "1.2.4", want "1.2.3"`) {
		t.Fatalf("error = %v", err)
	}
}

func TestEnsureAllRejectsPATHThatDoesNotSelectManagedExecutable(t *testing.T) {
	runtime := newFakeRuntime("future 1.2.3")
	runtime.executablePaths["future"] = "/legacy/bin/future"

	_, err := newTestInstaller(runtime, time.Second).EnsureAll(
		context.Background(),
		[]provisioning.Profile{testProfile(provisioning.InstallWithNPM)},
	)
	if err == nil || !strings.Contains(err.Error(), `managed executable "/managed/host-clis/bin/future"`) ||
		!strings.Contains(err.Error(), `PATH resolves "future" to "/legacy/bin/future"`) {
		t.Fatalf("error = %v", err)
	}
	if len(runtime.npmInstalls) != 0 {
		t.Fatalf("installer should not reinstall a current managed executable: %#v", runtime.npmInstalls)
	}
}

func TestEnsureAllMigratesLegacyPATHExecutableIntoManagedPrefix(t *testing.T) {
	runtime := newFakeRuntime("")
	runtime.executablePaths["future"] = "/usr/bin/future"
	runtime.versions["/usr/bin/future"] = "future 1.2.2"
	runtime.exists["/usr/bin/future"] = true
	runtime.installedOutput = "future 1.2.3"

	results, err := newTestInstaller(runtime, time.Second).EnsureAll(
		context.Background(),
		[]provisioning.Profile{testProfile(provisioning.InstallWithNPM)},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || !results[0].Changed || runtime.executablePaths["future"] != testManagedExecutable("future") {
		t.Fatalf("results = %#v, executable = %q", results, runtime.executablePaths["future"])
	}
}

func TestEnsureAllReturnsProviderScopedInstallFailure(t *testing.T) {
	runtime := newFakeRuntime("")
	runtime.installErr = errors.New("registry unavailable")
	_, err := newTestInstaller(runtime, time.Second).EnsureAll(context.Background(), []provisioning.Profile{testProfile(provisioning.InstallWithNPM)})
	if err == nil || !strings.Contains(err.Error(), `converge host agent "future-agent"`) ||
		!strings.Contains(err.Error(), "registry unavailable") {
		t.Fatalf("error = %v", err)
	}
}

func TestEnsureAllSupportsExistenceOnlyPolicies(t *testing.T) {
	profile := testProfile(provisioning.InstallWithNPM)
	profile.CLI.CheckVersion = false
	runtime := newFakeRuntime("")
	runtime.exists[testManagedExecutable("future")] = true
	runtime.executablePaths["future"] = testManagedExecutable("future")
	results, err := newTestInstaller(runtime, time.Second).EnsureAll(context.Background(), []provisioning.Profile{profile})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Changed {
		t.Fatalf("results = %#v", results)
	}
	if results[0].VersionChecked || results[0].DetectedVersion != "" {
		t.Fatalf("existence-only result claims a version check: %#v", results[0])
	}
}

func TestEnsureAllBoundsEachInstallation(t *testing.T) {
	profile := testProfile(provisioning.InstallWithNPM)
	profile.CLI.InstallTimeout = 20 * time.Millisecond
	runtime := newFakeRuntime("")
	runtime.waitForCancel = true
	started := time.Now()
	_, err := newTestInstaller(runtime, time.Second).EnsureAll(context.Background(), []provisioning.Profile{profile})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want deadline exceeded", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("timed installation took %s", elapsed)
	}
}

func TestEnsureAllBoundsVersionChecks(t *testing.T) {
	for name, testCase := range map[string]struct {
		blockedCall int
		wantError   bool
	}{
		"initial probe":             {blockedCall: 1},
		"post-install verification": {blockedCall: 2, wantError: true},
	} {
		t.Run(name, func(t *testing.T) {
			runtime := newFakeRuntime("future 1.2.2")
			runtime.installedOutput = "future 1.2.3"
			runtime.blockVersionCall = testCase.blockedCall
			installer := newTestInstaller(runtime, 20*time.Millisecond)
			started := time.Now()
			_, err := installer.EnsureAll(context.Background(), []provisioning.Profile{testProfile(provisioning.InstallWithNPM)})
			if (err != nil) != testCase.wantError {
				t.Fatalf("error = %v, want error %t", err, testCase.wantError)
			}
			if elapsed := time.Since(started); elapsed > time.Second {
				t.Fatalf("version-check failure took %s", elapsed)
			}
		})
	}
}

func TestEnsureAllRejectsNonPositiveVersionTimeout(t *testing.T) {
	_, err := New(newFakeRuntime("future 1.2.3"), 0, testManagedPrefix).EnsureAll(
		context.Background(),
		[]provisioning.Profile{testProfile(provisioning.InstallWithNPM)},
	)
	if err == nil || !strings.Contains(err.Error(), "version timeout must be positive") {
		t.Fatalf("error = %v", err)
	}
}

func TestEnsureAllRejectsUnsafeManagedPrefix(t *testing.T) {
	for _, prefix := range []string{"", ".", "/"} {
		_, err := New(newFakeRuntime(""), time.Second, prefix).EnsureAll(
			context.Background(),
			[]provisioning.Profile{testProfile(provisioning.InstallWithNPM)},
		)
		if err == nil || !strings.Contains(err.Error(), "managed prefix must be an absolute path below root") {
			t.Fatalf("prefix %q error = %v", prefix, err)
		}
	}
}

func TestEnsureAllRejectsBinaryOutsideManagedBinDirectory(t *testing.T) {
	for _, binary := range []string{"", ".", "..", "../future", "/usr/bin/future"} {
		profile := testProfile(provisioning.InstallWithNPM)
		profile.CLI.Binary = binary
		_, err := newTestInstaller(newFakeRuntime(""), time.Second).EnsureAll(
			context.Background(),
			[]provisioning.Profile{profile},
		)
		if err == nil || !strings.Contains(err.Error(), "binary must be a plain file name") {
			t.Fatalf("binary %q error = %v", binary, err)
		}
	}
}

func TestEnsureAllGivesPostInstallVerificationAnIndependentDeadline(t *testing.T) {
	runtime := newFakeRuntime("future 1.2.2")
	runtime.installedOutput = "future 1.2.3"
	runtime.installHook = func() {
		time.Sleep(150 * time.Millisecond)
	}
	profile := testProfile(provisioning.InstallWithNPM)
	profile.CLI.InstallTimeout = 200 * time.Millisecond
	installer := newTestInstaller(runtime, time.Second)

	if _, err := installer.EnsureAll(context.Background(), []provisioning.Profile{profile}); err != nil {
		t.Fatal(err)
	}
	if got := runtime.versionTimeout[1]; got < 500*time.Millisecond {
		t.Fatalf("post-install version timeout = %s, want an independent deadline", got)
	}
}

func TestMatchSemanticVersionRecognizesCLIOutputForms(t *testing.T) {
	for name, testCase := range map[string]struct {
		input        string
		want         string
		wantMatch    bool
		wantDetected string
	}{
		"plain":           {input: "claude 1.2.3 (Claude Code)", want: "1.2.3", wantMatch: true, wantDetected: "1.2.3"},
		"prerelease":      {input: "codex-cli 1.2.3-beta.1", want: "1.2.3-beta.1", wantMatch: true, wantDetected: "1.2.3-beta.1"},
		"v prefix":        {input: "v1.2.3", want: "1.2.3", wantMatch: true, wantDetected: "1.2.3"},
		"warning version": {input: "runtime 22.19.0 warning\nfuture 1.2.3", want: "1.2.3", wantMatch: true, wantDetected: "1.2.3"},
		"mismatch":        {input: "runtime 22.19.0 warning\nfuture 1.2.2", want: "1.2.3", wantDetected: "1.2.2"},
		"no version":      {input: "no version", want: "1.2.3"},
	} {
		t.Run(name, func(t *testing.T) {
			matched, detected := matchSemanticVersion(testCase.input, testCase.want)
			if matched != testCase.wantMatch || detected != testCase.wantDetected {
				t.Fatalf("matchSemanticVersion(%q, %q) = (%t, %q), want (%t, %q)",
					testCase.input, testCase.want, matched, detected, testCase.wantMatch, testCase.wantDetected)
			}
		})
	}
}

func newFakeRuntime(version string) *fakeRuntime {
	runtime := &fakeRuntime{
		exists:          make(map[string]bool),
		executablePaths: make(map[string]string),
		versions:        make(map[string]string),
	}
	if version != "" {
		executable := testManagedExecutable("future")
		runtime.exists[executable] = true
		runtime.executablePaths["future"] = executable
		runtime.versions[executable] = version
	}
	return runtime
}

func newTestInstaller(runtime Runtime, versionTimeout time.Duration) *Installer {
	return New(runtime, versionTimeout, testManagedPrefix)
}

func testManagedExecutable(binary string) string {
	return filepath.Join(testManagedPrefix, "bin", binary)
}

func testProfile(mode provisioning.InstallMode) provisioning.Profile {
	return provisioning.Profile{
		ID: "future-agent",
		CLI: provisioning.CLISpec{
			Name:               "Future Agent",
			ImageLabel:         "future-agent",
			Binary:             "future",
			VersionArgs:        []string{"version"},
			PackageName:        "future-cli",
			Version:            "1.2.3",
			ReportVersion:      true,
			CheckVersion:       true,
			VerifyAfterInstall: true,
			InstallMode:        mode,
			InstallTimeout:     time.Minute,
		},
	}
}
