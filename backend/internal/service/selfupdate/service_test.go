package selfupdate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

type fakeHost struct {
	tags     []string
	tagsErr  error
	started  []string
	kinds    []string
	pid      int
	alive    bool
	startErr error
}

func (f *fakeHost) ListRemoteTags(context.Context, string) ([]string, error) {
	return f.tags, f.tagsErr
}

func (f *fakeHost) StartUpdater(launch UpdaterLaunch) (int, error) {
	if f.startErr != nil {
		return 0, f.startErr
	}
	f.started = append(f.started, launch.Target)
	f.kinds = append(f.kinds, string(launch.Kind))
	return f.pid, nil
}

func (f *fakeHost) ProcessAlive(int) bool { return f.alive }

func TestParseReleaseTag(t *testing.T) {
	cases := []struct {
		tag    string
		want   []int
		wantOK bool
	}{
		{"0.1", []int{0, 1}, true},
		{"v0.2.3", []int{0, 2, 3}, true},
		{"1", []int{1}, true},
		{"dev", nil, false},
		{"db01776", nil, false},
		{"v", nil, false},
		{"0.1-rc1", nil, false},
		{"", nil, false},
	}
	for _, c := range cases {
		got, ok := parseReleaseTag(c.tag)
		if ok != c.wantOK {
			t.Errorf("parseReleaseTag(%q) ok = %v, want %v", c.tag, ok, c.wantOK)
			continue
		}
		if !ok {
			continue
		}
		if compareVersions(got, c.want) != 0 {
			t.Errorf("parseReleaseTag(%q) = %v, want %v", c.tag, got, c.want)
		}
	}
}

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"0.1", "0.2", -1},
		{"0.2", "0.1", 1},
		{"0.1", "0.1", 0},
		{"0.1", "0.1.0", 0},
		{"0.1.1", "0.1", 1},
		{"1.0", "0.9.9", 1},
		{"0.10", "0.9", 1},
	}
	for _, c := range cases {
		a, _ := parseReleaseTag(c.a)
		b, _ := parseReleaseTag(c.b)
		if got := compareVersions(a, b); got != c.want {
			t.Errorf("compareVersions(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestClassifyUpdate(t *testing.T) {
	cases := []struct {
		name    string
		current string
		target  string
		want    UpdateKind
	}{
		{"patch release", "0.3.1", "0.3.2", UpdateKindApplication},
		{"commit after patch release", "0.3.1-12-gdb01776", "0.3.2", UpdateKindApplication},
		{"legacy fourth segment", "0.3.1", "0.3.1.1", UpdateKindApplication},
		{"minor release", "0.3.1", "0.4.0", UpdateKindInfrastructure},
		{"skips minor baseline", "0.3.1", "0.4.2", UpdateKindInfrastructure},
		{"already on minor baseline", "0.4.0", "0.4.2", UpdateKindApplication},
		{"major release", "1.9.5", "2.0.0", UpdateKindInfrastructure},
		{"legacy two-part version", "0.3", "0.3.1", UpdateKindInfrastructure},
		{"unstamped build", "dev", "0.3.2", UpdateKindInfrastructure},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := classifyUpdate(c.current, c.target); got != c.want {
				t.Fatalf("classifyUpdate(%q, %q) = %q, want %q", c.current, c.target, got, c.want)
			}
		})
	}
}

func TestLatestReleaseTag(t *testing.T) {
	tag, _ := latestReleaseTag([]string{"0.1", "v0.10", "0.9", "nightly", "db01776"})
	if tag != "v0.10" {
		t.Fatalf("latestReleaseTag = %q, want v0.10", tag)
	}
	if tag, _ := latestReleaseTag([]string{"nightly"}); tag != "" {
		t.Fatalf("latestReleaseTag(no releases) = %q, want empty", tag)
	}
}

func TestCheckComparesAgainstDescribeOutput(t *testing.T) {
	host := &fakeHost{tags: []string{"0.1", "0.2"}}
	cases := []struct {
		current string
		want    bool
	}{
		{"0.1", true},
		{"0.1-12-gdb01776", true}, // main past 0.1, release 0.2 is still newer news
		{"0.2", false},
		{"0.2-3-gabc1234", false},
		{"0.3", false},
		{"dev", false}, // unstamped build: cannot claim anything
	}
	for _, c := range cases {
		svc := New(c.current, "/opt/x", t.TempDir(), host)
		status := svc.Check(context.Background())
		if status.LastCheck == nil {
			t.Fatalf("current=%q: no check result", c.current)
		}
		if status.LastCheck.UpdateAvailable != c.want {
			t.Errorf("current=%q: updateAvailable = %v, want %v",
				c.current, status.LastCheck.UpdateAvailable, c.want)
		}
		if status.LastCheck.LatestTag != "0.2" {
			t.Errorf("current=%q: latestTag = %q, want 0.2", c.current, status.LastCheck.LatestTag)
		}
		if status.LastCheck.UpdateAvailable && status.LastCheck.UpdateKind != UpdateKindInfrastructure {
			t.Errorf("current=%q: updateKind = %q, want infrastructure for legacy two-part tag", c.current, status.LastCheck.UpdateKind)
		}
	}
}

func TestCheckReportsApplicationUpdateWithinReleaseLine(t *testing.T) {
	host := &fakeHost{tags: []string{"0.3.1", "0.3.2"}}
	status := New("0.3.1", "/opt/x", t.TempDir(), host).Check(context.Background())
	if status.LastCheck == nil || status.LastCheck.UpdateKind != UpdateKindApplication {
		t.Fatalf("last check = %+v, want application update", status.LastCheck)
	}
}

func TestApplyLifecycle(t *testing.T) {
	host := &fakeHost{tags: []string{"0.1", "0.2"}, pid: 4242, alive: true}
	svc := New("0.1", "/opt/x", t.TempDir(), host)

	status, err := svc.Apply(context.Background(), "admin@example.com", "")
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(host.started) != 1 || host.started[0] != "0.2" {
		t.Fatalf("started = %v, want [0.2]", host.started)
	}
	if len(host.kinds) != 1 || host.kinds[0] != string(UpdateKindInfrastructure) {
		t.Fatalf("update kinds = %v, want [infrastructure]", host.kinds)
	}
	if status.Run == nil || status.Run.State != "running" || status.Run.Target != "0.2" {
		t.Fatalf("run status = %+v, want running 0.2", status.Run)
	}

	// Second apply while the first is alive must refuse.
	if _, err := svc.Apply(context.Background(), "admin@example.com", ""); !errors.Is(err, ErrUpdateInProgress) {
		t.Fatalf("second Apply err = %v, want ErrUpdateInProgress", err)
	}

	// Process death without a done marker reads as failure.
	host.alive = false
	if run := svc.Status(context.Background()).Run; run == nil || run.State != "failed" {
		t.Fatalf("run after crash = %+v, want failed", run)
	}

	// A finished marker wins over liveness.
	if err := writeJSONFile(svc.runs.donePath(), doneRecord{ExitCode: 0, FinishedAt: 99}); err != nil {
		t.Fatal(err)
	}
	if run := svc.Status(context.Background()).Run; run == nil || run.State != "succeeded" {
		t.Fatalf("run after done marker = %+v, want succeeded", run)
	}
}

func TestRunStatusReportsCleanLogAndStructuredProgress(t *testing.T) {
	host := &fakeHost{tags: []string{"0.1", "0.2"}, pid: 4242, alive: true}
	svc := New("0.1", "/opt/x", t.TempDir(), host)
	if _, err := svc.Apply(context.Background(), "admin@example.com", "0.2"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(svc.runs.logPath(), []byte("\x1b[1;36m==> Installing healer\x1b[0m\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	wantProgress := Progress{
		Phase: "workspace-migration", Message: "Recycling workspace 3 of 8",
		Completed: 2, Total: 8, CurrentItem: "astrology", UpdatedAt: 123,
	}
	if err := writeJSONFile(svc.runs.progressPath(), wantProgress); err != nil {
		t.Fatal(err)
	}

	run := svc.Status(context.Background()).Run
	if run == nil {
		t.Fatal("run status is nil")
	}
	if run.Log != "==> Installing healer\n" {
		t.Fatalf("clean log = %q", run.Log)
	}
	if run.LogUpdatedAt == 0 {
		t.Fatal("log update timestamp is missing")
	}
	if run.Progress == nil || *run.Progress != wantProgress {
		t.Fatalf("progress = %+v, want %+v", run.Progress, wantProgress)
	}
}

func TestApplyStartsApplicationDeploymentWithinReleaseLine(t *testing.T) {
	host := &fakeHost{tags: []string{"0.3.1", "0.3.2"}, pid: 4242, alive: true}
	svc := New("0.3.1", "/opt/x", t.TempDir(), host)
	status, err := svc.Apply(context.Background(), "admin@example.com", "0.3.2")
	if err != nil {
		t.Fatal(err)
	}
	if len(host.kinds) != 1 || host.kinds[0] != string(UpdateKindApplication) {
		t.Fatalf("update kinds = %v, want [application]", host.kinds)
	}
	if status.Run == nil || status.Run.UpdateKind != UpdateKindApplication {
		t.Fatalf("run = %+v, want application update", status.Run)
	}
}

func TestApplyValidatesTag(t *testing.T) {
	host := &fakeHost{tags: []string{"0.1"}}
	svc := New("0.1", "/opt/x", t.TempDir(), host)
	if _, err := svc.Apply(context.Background(), "a@b.c", "0.9"); !errors.Is(err, ErrUnknownTag) {
		t.Fatalf("Apply(unknown tag) err = %v, want ErrUnknownTag", err)
	}
	host.tags = []string{"nightly"}
	if _, err := svc.Apply(context.Background(), "a@b.c", ""); !errors.Is(err, ErrNoReleaseTag) {
		t.Fatalf("Apply(no releases) err = %v, want ErrNoReleaseTag", err)
	}
}

// TestApplyRetryPreservesInfrastructureKind simulates a failed infrastructure
// update where the backend binary has already been replaced by the new
// release. Re-applying toward the same target must keep the host convergence
// path that actually failed, even though classifying against the running
// version would otherwise collapse the run to an application-only deploy.
func TestApplyRetryPreservesInfrastructureKind(t *testing.T) {
	host := &fakeHost{tags: []string{"0.11.0", "0.12.0"}, pid: 4242}
	svc := New("0.11.0", "/opt/x", t.TempDir(), host)

	if _, err := svc.Apply(context.Background(), "admin@example.com", "0.12.0"); err != nil {
		t.Fatalf("first Apply: %v", err)
	}
	if got := host.kinds[len(host.kinds)-1]; got != string(UpdateKindInfrastructure) {
		t.Fatalf("first run kind = %s, want infrastructure", got)
	}

	// Mark the in-flight run as failed by clearing liveness and dropping a
	// done marker. The Service is restarted to mimic the backend restart
	// that the failing updater triggered before the partial install settled.
	host.alive = false
	if err := writeJSONFile(svc.runs.donePath(), doneRecord{ExitCode: 1, FinishedAt: 1}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSONFile(svc.runs.runPath(), runRecord{
		Target: "0.12.0", UpdateKind: UpdateKindInfrastructure,
		StartedAt: 1, StartedBy: "admin@example.com", PID: 4242,
	}); err != nil {
		t.Fatal(err)
	}

	// New binary is now reporting 0.12.0 because the previous infrastructure
	// step rebuilt it before failing. A naive classification would now
	// return application; the retry must instead re-use the failed kind.
	svc2 := New("0.12.0", "/opt/x", filepath.Dir(svc.runs.dir), host)
	host.alive = true
	if _, err := svc2.Apply(context.Background(), "admin@example.com", "0.12.0"); err != nil {
		t.Fatalf("retry Apply: %v", err)
	}
	if got := host.kinds[len(host.kinds)-1]; got != string(UpdateKindInfrastructure) {
		t.Fatalf("retry kind = %s, want infrastructure (preserved from failed run)", got)
	}
}

// TestApplyRetryReclassifiesWhenTargetChanges confirms that re-applying toward
// a different tag is free to pick up a fresh classification. Only a retry
// against the exact same target reuses the failed run's kind.
func TestApplyRetryReclassifiesWhenTargetChanges(t *testing.T) {
	host := &fakeHost{tags: []string{"0.11.0", "0.12.0"}, pid: 4242}
	svc := New("0.11.0", "/opt/x", t.TempDir(), host)

	if _, err := svc.Apply(context.Background(), "admin@example.com", "0.12.0"); err != nil {
		t.Fatal(err)
	}
	host.alive = false
	if err := writeJSONFile(svc.runs.donePath(), doneRecord{ExitCode: 1, FinishedAt: 1}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSONFile(svc.runs.runPath(), runRecord{
		Target: "0.12.0", UpdateKind: UpdateKindInfrastructure,
		StartedAt: 1, StartedBy: "admin@example.com", PID: 4242,
	}); err != nil {
		t.Fatal(err)
	}
	host.alive = true

	if _, err := svc.Apply(context.Background(), "admin@example.com", "0.11.0"); err != nil {
		t.Fatal(err)
	}
	if got := host.kinds[len(host.kinds)-1]; got != string(UpdateKindApplication) {
		t.Fatalf("downgrade kind = %s, want application (no preservation across targets)", got)
	}
}

// TestApplyStartUpdaterFailureClearsStaleRecord pins down the contract that
// reset() + removeRecord() together guarantee: a launch that never made it
// past StartUpdater must not leave a half-written run.json behind for
// Status() to resurrect as a phantom failed run. Without this, an admin
// could see the previous target's stale record with an empty log and no
// progress, and assume the new attempt had failed.
func TestApplyStartUpdaterFailureClearsStaleRecord(t *testing.T) {
	host := &fakeHost{tags: []string{"0.11.0", "0.12.0"}, pid: 4242}
	svc := New("0.11.0", "/opt/x", t.TempDir(), host)

	// Land a prior failed infrastructure run on disk so the retry starts
	// from the realistic partial-install state.
	if _, err := svc.Apply(context.Background(), "admin@example.com", "0.12.0"); err != nil {
		t.Fatal(err)
	}
	host.alive = false
	if err := writeJSONFile(svc.runs.donePath(), doneRecord{ExitCode: 1, FinishedAt: 1}); err != nil {
		t.Fatal(err)
	}

	// Now retry. StartUpdater fails; Apply must clear the previous run.json.
	host.startErr = errors.New("synthetic launch failure")
	host.alive = true
	status, err := svc.Apply(context.Background(), "admin@example.com", "0.12.0")
	if err == nil {
		t.Fatalf("Apply err = nil, want synthetic launch failure")
	}
	if status.Run != nil {
		t.Fatalf("status.Run = %+v, want nil so the UI does not show a phantom failed run", status.Run)
	}
	if _, err := os.Stat(svc.runs.runPath()); !os.IsNotExist(err) {
		t.Fatalf("run.json still present after StartUpdater failure: err=%v", err)
	}
	if _, err := os.Stat(svc.runs.progressPath()); !os.IsNotExist(err) {
		t.Fatalf("progress.json still present after StartUpdater failure: err=%v", err)
	}
}
