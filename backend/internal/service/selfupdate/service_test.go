package selfupdate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

type fakeHost struct {
	tags        []string
	tagsErr     error
	commitTags  []string
	commitErr   error
	commitQuery string
	commitCalls int
	notesBody   string
	notesErr    error
	notesCalls  int
	started     []string
	kinds       []string
	pid         int
	alive       bool
	startErr    error
	beforeStart func()
}

type updateLifecycleEvent struct {
	state     string
	target    string
	kind      string
	startedBy string
}

type recordingUpdateLifecyclePublisher struct {
	events []updateLifecycleEvent
}

func (p *recordingUpdateLifecyclePublisher) PublishUpdateStarted(_ context.Context, target, kind, startedBy string) {
	p.record("started", target, kind, startedBy)
}

func (p *recordingUpdateLifecyclePublisher) PublishUpdateSucceeded(_ context.Context, target, kind, startedBy string) {
	p.record("succeeded", target, kind, startedBy)
}

func (p *recordingUpdateLifecyclePublisher) PublishUpdateFailed(_ context.Context, target, kind, startedBy string) {
	p.record("failed", target, kind, startedBy)
}

func (p *recordingUpdateLifecyclePublisher) record(state, target, kind, startedBy string) {
	p.events = append(p.events, updateLifecycleEvent{state: state, target: target, kind: kind, startedBy: startedBy})
}

type noopUpdateLifecyclePublisher struct{}

func (noopUpdateLifecyclePublisher) PublishUpdateStarted(context.Context, string, string, string) {}
func (noopUpdateLifecyclePublisher) PublishUpdateSucceeded(context.Context, string, string, string) {
}
func (noopUpdateLifecyclePublisher) PublishUpdateFailed(context.Context, string, string, string) {}

func newTestService(currentVersion, installDir, dataDir string, host HostClient) *Service {
	return New(currentVersion, installDir, dataDir, host, noopUpdateLifecyclePublisher{})
}

func (f *fakeHost) ListRemoteTags(context.Context, string) ([]string, error) {
	return f.tags, f.tagsErr
}

func (f *fakeHost) ListRemoteTagsForCommit(_ context.Context, _, commitPrefix string) ([]string, error) {
	f.commitQuery = commitPrefix
	f.commitCalls++
	return f.commitTags, f.commitErr
}

func (f *fakeHost) ReadReleaseNotes(context.Context, string, string) (string, error) {
	f.notesCalls++
	return f.notesBody, f.notesErr
}

func (f *fakeHost) StartUpdater(launch UpdaterLaunch) (int, error) {
	if f.beforeStart != nil {
		f.beforeStart()
	}
	if f.startErr != nil {
		return 0, f.startErr
	}
	f.started = append(f.started, launch.Target)
	f.kinds = append(f.kinds, string(launch.Kind))
	return f.pid, nil
}

func (f *fakeHost) ProcessAlive(int) bool { return f.alive }

func TestReleaseNotesDoesNotChangeUpdateAvailability(t *testing.T) {
	host := &fakeHost{tags: []string{"0.25.1"}, notesErr: errors.New("origin unavailable")}
	svc := newTestService("0.25.0", "/opt/remote", t.TempDir(), host)
	before := svc.Check(context.Background())
	if _, err := svc.ReleaseNotes(context.Background(), "0.25.1"); err == nil {
		t.Fatal("expected notes read failure")
	}
	after := svc.Status(context.Background())
	if after.LastCheck != before.LastCheck || !after.LastCheck.UpdateAvailable {
		t.Fatal("notes failure changed update availability")
	}
	host.notesErr = nil
	host.notesBody = "## Fixes\n- Preserve project files."
	notes, err := svc.ReleaseNotes(context.Background(), "0.25.1")
	if err != nil || notes.Tag != "0.25.1" || notes.Body != host.notesBody {
		t.Fatalf("retry notes = %+v, error = %v", notes, err)
	}
}

func TestReleaseNotesRejectsInvalidTagsBeforeReading(t *testing.T) {
	host := &fakeHost{}
	svc := newTestService("0.25.0", "/opt/remote", t.TempDir(), host)
	for _, tag := range []string{"", "main", "../main", "--help", "v0.25.1/other", "0.25.1:refs/heads/main"} {
		if _, err := svc.ReleaseNotes(context.Background(), tag); !errors.Is(err, ErrInvalidReleaseTag) {
			t.Errorf("tag %q: error = %v", tag, err)
		}
	}
	if host.notesCalls != 0 {
		t.Fatalf("reader called %d times for invalid tags", host.notesCalls)
	}
}

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
		svc := newTestService(c.current, "/opt/x", t.TempDir(), host)
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
	status := newTestService("0.3.1", "/opt/x", t.TempDir(), host).Check(context.Background())
	if status.LastCheck == nil || status.LastCheck.UpdateKind != UpdateKindApplication {
		t.Fatalf("last check = %+v, want application update", status.LastCheck)
	}
}

func TestHashStampedReleaseUsesResolvedBaselineForCheckAndApply(t *testing.T) {
	for _, test := range []struct {
		name           string
		currentVersion string
		commitQuery    string
		tags           []string
		baseline       string
		target         string
		wantKind       UpdateKind
	}{
		{
			name: "bare hash in same release line", currentVersion: "42989fe", commitQuery: "42989fe",
			tags: []string{"0.20.1", "0.20.3", "0.20.4"}, baseline: "0.20.1", target: "0.20.4",
			wantKind: UpdateKindApplication,
		},
		{
			name: "bare hash across minor boundary", currentVersion: "42989fe", commitQuery: "42989fe",
			tags: []string{"0.20.4", "0.21.0"}, baseline: "0.20.4", target: "0.21.0",
			wantKind: UpdateKindInfrastructure,
		},
		{
			name: "QA candidate", currentVersion: "qa-6db1ea1ade2a", commitQuery: "6db1ea1ade2a",
			tags: []string{"0.20.4", "0.21.0"}, baseline: "0.20.4", target: "0.21.0",
			wantKind: UpdateKindInfrastructure,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			host := &fakeHost{
				tags: test.tags, commitTags: []string{test.baseline}, pid: 4242, alive: true,
			}
			svc := newTestService(test.currentVersion, "/opt/x", t.TempDir(), host)

			status := svc.Check(context.Background())
			if host.commitQuery != test.commitQuery {
				t.Fatalf("commit lookup = %q, want %q", host.commitQuery, test.commitQuery)
			}
			if status.LastCheck == nil || status.LastCheck.Error != "" || !status.LastCheck.UpdateAvailable ||
				status.LastCheck.LatestTag != test.target || status.LastCheck.UpdateKind != test.wantKind {
				t.Fatalf("last check = %+v, want %s update to %s", status.LastCheck, test.wantKind, test.target)
			}

			status, err := svc.Apply(context.Background(), "admin@example.com", test.target)
			if err != nil {
				t.Fatalf("Apply: %v", err)
			}
			if host.commitCalls != 1 {
				t.Fatalf("commit lookups = %d, want 1 shared by Check and Apply", host.commitCalls)
			}
			if len(host.kinds) != 1 || host.kinds[0] != string(test.wantKind) {
				t.Fatalf("update kinds = %v, want [%s]", host.kinds, test.wantKind)
			}
			if status.Run == nil || status.Run.UpdateKind != test.wantKind {
				t.Fatalf("run = %+v, want %s update", status.Run, test.wantKind)
			}
		})
	}
}

func TestCheckUsesEmbeddedQACandidateBaseline(t *testing.T) {
	host := &fakeHost{tags: []string{"0.20.4", "0.21.0"}}
	status := newTestService("qa-0.20.4-6db1ea1ade2a", "/opt/x", t.TempDir(), host).Check(context.Background())
	if host.commitCalls != 0 {
		t.Fatalf("commit lookups = %d, want none for embedded baseline", host.commitCalls)
	}
	if status.LastCheck == nil || status.LastCheck.Error != "" || !status.LastCheck.UpdateAvailable ||
		status.LastCheck.LatestTag != "0.21.0" || status.LastCheck.UpdateKind != UpdateKindInfrastructure {
		t.Fatalf("last check = %+v, want infrastructure update from 0.20.4 to 0.21.0", status.LastCheck)
	}
}

func TestCommitFromVersionRecognizesReleaseCandidatesOnly(t *testing.T) {
	for _, test := range []struct {
		version string
		want    string
		ok      bool
	}{
		{version: "42989fe", want: "42989fe", ok: true},
		{version: "qa-6db1ea1ade2a", want: "6db1ea1ade2a", ok: true},
		{version: "qa-0.20.4-6db1ea1ade2a", want: "6db1ea1ade2a", ok: true},
		{version: "qa-local-6db1ea1ade2a-clean-20260927", ok: false},
		{version: "qa-", ok: false},
		{version: "dev", ok: false},
	} {
		t.Run(test.version, func(t *testing.T) {
			got, ok := commitFromVersion(test.version)
			if got != test.want || ok != test.ok {
				t.Fatalf("commitFromVersion(%q) = (%q, %v), want (%q, %v)", test.version, got, ok, test.want, test.ok)
			}
		})
	}
}

func TestCheckDoesNotClaimUnknownCommitIsUpToDate(t *testing.T) {
	for _, currentVersion := range []string{"42989fe", "qa-6db1ea1ade2a"} {
		t.Run(currentVersion, func(t *testing.T) {
			host := &fakeHost{tags: []string{"0.20.4"}}
			status := newTestService(currentVersion, "/opt/x", t.TempDir(), host).Check(context.Background())
			if status.LastCheck == nil || status.LastCheck.Error == "" || status.LastCheck.UpdateAvailable {
				t.Fatalf("last check = %+v, want a version resolution error", status.LastCheck)
			}
		})
	}
}

func TestApplyUsesInfrastructurePathWhenHashCannotBeResolved(t *testing.T) {
	for _, test := range []struct {
		name      string
		commitErr error
	}{
		{name: "untagged"},
		{name: "ambiguous", commitErr: errors.New("commit prefix matches multiple tagged commits")},
	} {
		t.Run(test.name, func(t *testing.T) {
			host := &fakeHost{
				tags: []string{"0.20.4"}, commitErr: test.commitErr, pid: 4242, alive: true,
			}
			svc := newTestService("42989fe", "/opt/x", t.TempDir(), host)
			status, err := svc.Apply(context.Background(), "admin@example.com", "0.20.4")
			if err != nil {
				t.Fatalf("Apply: %v", err)
			}
			if len(host.kinds) != 1 || host.kinds[0] != string(UpdateKindInfrastructure) {
				t.Fatalf("update kinds = %v, want [infrastructure]", host.kinds)
			}
			if status.Run == nil || status.Run.UpdateKind != UpdateKindInfrastructure {
				t.Fatalf("run = %+v, want infrastructure update", status.Run)
			}
		})
	}
}

func TestApplyLifecycle(t *testing.T) {
	lifecycle := &recordingUpdateLifecyclePublisher{}
	host := &fakeHost{tags: []string{"0.1", "0.2"}, pid: 4242, alive: true}
	host.beforeStart = func() {
		if len(lifecycle.events) != 1 || lifecycle.events[0].state != "started" {
			t.Fatalf("updater launched before update-started dispatch: %+v", lifecycle.events)
		}
	}
	svc := New("0.1", "/opt/x", t.TempDir(), host, lifecycle)

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
	wantEvent := updateLifecycleEvent{state: "started", target: "0.2", kind: string(UpdateKindInfrastructure), startedBy: "admin@example.com"}
	if len(lifecycle.events) != 1 || lifecycle.events[0] != wantEvent {
		t.Fatalf("update-started events = %+v, want [%+v]", lifecycle.events, wantEvent)
	}

	// Second apply while the first is alive must refuse.
	if _, err := svc.Apply(context.Background(), "admin@example.com", ""); !errors.Is(err, ErrUpdateInProgress) {
		t.Fatalf("second Apply err = %v, want ErrUpdateInProgress", err)
	}
	if len(lifecycle.events) != 1 {
		t.Fatalf("in-progress Apply published another event: %+v", lifecycle.events)
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
	svc := newTestService("0.1", "/opt/x", t.TempDir(), host)
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

func TestReconcileLifecycleCheckpointsTerminalEventAcrossRestart(t *testing.T) {
	for _, test := range []struct {
		name      string
		exitCode  int
		wantState string
	}{
		{name: "succeeded", exitCode: 0, wantState: "succeeded"},
		{name: "failed", exitCode: 1, wantState: "failed"},
	} {
		t.Run(test.name, func(t *testing.T) {
			dataDir := t.TempDir()
			host := &fakeHost{}
			publisher := &recordingUpdateLifecyclePublisher{}
			svc := New("0.4.0", "/opt/x", dataDir, host, publisher)
			record := runRecord{
				Target: "0.5.0", UpdateKind: UpdateKindInfrastructure,
				StartedAt: 10, StartedBy: "admin@example.com", PID: 4242,
			}
			if err := svc.runs.reset(); err != nil {
				t.Fatal(err)
			}
			if err := svc.runs.writeRecord(record); err != nil {
				t.Fatal(err)
			}
			if err := writeJSONFile(svc.runs.donePath(), doneRecord{ExitCode: test.exitCode, FinishedAt: 20}); err != nil {
				t.Fatal(err)
			}

			if err := svc.reconcileLifecycle(context.Background()); err != nil {
				t.Fatalf("reconcileLifecycle: %v", err)
			}
			want := updateLifecycleEvent{
				state: test.wantState, target: "0.5.0",
				kind: string(UpdateKindInfrastructure), startedBy: "admin@example.com",
			}
			if len(publisher.events) != 1 || publisher.events[0] != want {
				t.Fatalf("events = %+v, want [%+v]", publisher.events, want)
			}

			// The marker is durable: neither another pass in this process nor a
			// newly constructed replacement service publishes the event again.
			if err := svc.reconcileLifecycle(context.Background()); err != nil {
				t.Fatal(err)
			}
			restartedPublisher := &recordingUpdateLifecyclePublisher{}
			restarted := New("0.5.0", "/opt/x", dataDir, host, restartedPublisher)
			if err := restarted.reconcileLifecycle(context.Background()); err != nil {
				t.Fatal(err)
			}
			if len(publisher.events) != 1 || len(restartedPublisher.events) != 0 {
				t.Fatalf("terminal event was duplicated: first=%+v restarted=%+v", publisher.events, restartedPublisher.events)
			}
		})
	}
}

func TestApplyStartsApplicationDeploymentWithinReleaseLine(t *testing.T) {
	host := &fakeHost{tags: []string{"0.3.1", "0.3.2"}, pid: 4242, alive: true}
	svc := newTestService("0.3.1", "/opt/x", t.TempDir(), host)
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
	svc := newTestService("0.1", "/opt/x", t.TempDir(), host)
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
	svc := newTestService("0.11.0", "/opt/x", t.TempDir(), host)

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
	svc2 := newTestService("0.12.0", "/opt/x", filepath.Dir(svc.runs.dir), host)
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
	svc := newTestService("0.11.0", "/opt/x", t.TempDir(), host)

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
	lifecycle := &recordingUpdateLifecyclePublisher{}
	svc := New("0.11.0", "/opt/x", t.TempDir(), host, lifecycle)

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
	wantStates := []string{"started", "started", "failed"}
	if len(lifecycle.events) != len(wantStates) {
		t.Fatalf("lifecycle events = %+v, want states %v", lifecycle.events, wantStates)
	}
	for index, want := range wantStates {
		if lifecycle.events[index].state != want {
			t.Fatalf("lifecycle event states = %+v, want %v", lifecycle.events, wantStates)
		}
	}
}
