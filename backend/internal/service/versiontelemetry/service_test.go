package versiontelemetry

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type memoryRepository struct {
	mu      sync.Mutex
	id      string
	attempt Attempt
	found   bool
	err     error
}

func (r *memoryRepository) InstallationID() (string, error) {
	return r.id, r.err
}

func (r *memoryRepository) LastAttempt() (Attempt, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.attempt, r.found, r.err
}

func (r *memoryRepository) SaveAttempt(attempt Attempt) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.err != nil {
		return r.err
	}
	r.attempt = attempt
	r.found = true
	return nil
}

type recordingReporter struct {
	mu      sync.Mutex
	reports []Report
	errors  []error
	calls   chan struct{}
}

func (r *recordingReporter) ReportVersion(_ context.Context, report Report) error {
	r.mu.Lock()
	r.reports = append(r.reports, report)
	var err error
	if len(r.errors) > 0 {
		err = r.errors[0]
		r.errors = r.errors[1:]
	}
	r.mu.Unlock()
	if r.calls != nil {
		r.calls <- struct{}{}
	}
	return err
}

type controlledWait struct {
	durations chan time.Duration
	advance   chan struct{}
}

func newControlledWait() *controlledWait {
	return &controlledWait{
		durations: make(chan time.Duration, 3),
		advance:   make(chan struct{}, 3),
	}
}

func (w *controlledWait) wait(ctx context.Context, duration time.Duration) bool {
	w.durations <- duration
	select {
	case <-ctx.Done():
		return false
	case <-w.advance:
		return true
	}
}

type reporterFunc func(context.Context, Report) error

func (f reporterFunc) ReportVersion(ctx context.Context, report Report) error {
	return f(ctx, report)
}

func TestRunUsesWeeklyIntervalAfterFailureAndSuccess(t *testing.T) {
	repo := &memoryRepository{id: "installation-id"}
	reporter := &recordingReporter{
		errors: []error{errors.New("telemetry unavailable")},
		calls:  make(chan struct{}, 3),
	}
	waits := newControlledWait()
	now := time.Date(2026, time.September, 27, 12, 0, 0, 0, time.UTC)
	service := newService("42989fe", repo, reporter, waits.wait, func() time.Time { return now })
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		service.run(ctx)
		close(done)
	}()

	waitForCall(t, reporter.calls)
	waitForDuration(t, waits.durations, reportInterval)
	now = now.Add(reportInterval)
	waits.advance <- struct{}{}
	waitForCall(t, reporter.calls)
	waitForDuration(t, waits.durations, reportInterval)
	cancel()
	waitForDone(t, done)

	reporter.mu.Lock()
	defer reporter.mu.Unlock()
	if len(reporter.reports) != 2 {
		t.Fatalf("reports = %v, want two", reporter.reports)
	}
	for _, report := range reporter.reports {
		if report.InstallationID != "installation-id" || report.Version != "42989fe" {
			t.Fatalf("report = %+v", report)
		}
	}
}

func TestReportIsWeeklyAcrossRestartsAndImmediateForNewVersion(t *testing.T) {
	repo := &memoryRepository{id: "installation-id"}
	reporter := &recordingReporter{errors: []error{errors.New("collector unavailable")}}
	start := time.Date(2026, time.September, 27, 12, 0, 0, 0, time.UTC)

	reportAt := func(version string, now time.Time) {
		service := newService(version, repo, reporter, nil, func() time.Time { return now })
		service.report(context.Background())
	}
	reportAt("0.21.0", start)
	reportAt("0.21.0", start.Add(24*time.Hour))
	reportAt("0.21.1", start.Add(48*time.Hour))
	reportAt("0.21.1", start.Add(8*24*time.Hour))
	reportAt("0.21.1", start.Add(9*24*time.Hour))

	reporter.mu.Lock()
	defer reporter.mu.Unlock()
	want := []string{"0.21.0", "0.21.1", "0.21.1"}
	if len(reporter.reports) != len(want) {
		t.Fatalf("reports = %v, want versions %v", reporter.reports, want)
	}
	for index, version := range want {
		if reporter.reports[index].Version != version {
			t.Fatalf("report %d version = %q, want %q", index, reporter.reports[index].Version, version)
		}
	}
}

func TestStartIsIdempotentAndCancellable(t *testing.T) {
	calls := make(chan struct{}, 2)
	service := New(
		"0.21.0",
		&memoryRepository{id: "installation-id"},
		reporterFunc(func(context.Context, Report) error {
			calls <- struct{}{}
			return nil
		}),
	)
	ctx, cancel := context.WithCancel(context.Background())

	service.Start(ctx)
	service.Start(ctx)
	waitForCall(t, calls)
	cancel()
	select {
	case <-calls:
		t.Fatal("a second Start launched another reporting loop")
	case <-time.After(20 * time.Millisecond):
	}
}

func TestRunWithCancelledContextDoesNotReport(t *testing.T) {
	calls := make(chan struct{}, 1)
	service := New(
		"0.21.0",
		&memoryRepository{id: "installation-id"},
		reporterFunc(func(context.Context, Report) error {
			calls <- struct{}{}
			return nil
		}),
	)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	service.run(ctx)
	select {
	case <-calls:
		t.Fatal("cancelled service reported a version")
	default:
	}
}

func TestAutomaticReportingVersionPolicy(t *testing.T) {
	tests := []struct {
		version string
		want    bool
	}{
		{version: "dev", want: false},
		{version: "qa-local-42989fe-clean-20260927", want: false},
		{version: "qa-local-", want: false},
		{version: "qa-42989fe", want: false},
		{version: "qa-release-candidate", want: false},
		{version: "qa-local", want: false},
		{version: "qa", want: true},
		{version: "qatest", want: true},
		{version: "Dev", want: true},
		{version: "dev-dirty", want: true},
		{version: "42989fe", want: true},
		{version: "0.20.1-3-g42989fe", want: true},
		{version: "0.21.0", want: true},
	}
	for _, test := range tests {
		if got := shouldReportVersion(test.version); got != test.want {
			t.Errorf("shouldReportVersion(%q) = %v, want %v", test.version, got, test.want)
		}
	}
}

func TestStartSkipsDevelopmentBuilds(t *testing.T) {
	for _, version := range []string{"dev", "qa-local-42989fe-clean-20260927", "qa-42989fe"} {
		t.Run(version, func(t *testing.T) {
			calls := make(chan struct{}, 1)
			service := New(
				version,
				&memoryRepository{id: "installation-id"},
				reporterFunc(func(context.Context, Report) error {
					calls <- struct{}{}
					return nil
				}),
			)
			service.Start(context.Background())
			select {
			case <-calls:
				t.Fatalf("development version %q was reported", version)
			case <-time.After(20 * time.Millisecond):
			}
		})
	}
}

func waitForCall(t *testing.T, calls <-chan struct{}) {
	t.Helper()
	select {
	case <-calls:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for telemetry report")
	}
}

func waitForDuration(t *testing.T, durations <-chan time.Duration, want time.Duration) {
	t.Helper()
	select {
	case got := <-durations:
		if got != want {
			t.Fatalf("next report delay = %s, want %s", got, want)
		}
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for %s report delay", want)
	}
}

func waitForDone(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for telemetry loop to stop")
	}
}
