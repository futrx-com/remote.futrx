// Package versiontelemetry periodically reports the running application
// version without coupling telemetry availability to server startup or the
// self-update workflow.
package versiontelemetry

import (
	"context"
	"strings"
	"sync"
	"time"
)

const reportInterval = 7 * 24 * time.Hour

type waitFunc func(context.Context, time.Duration) bool

type Service struct {
	version  string
	repo     Repository
	reporter Reporter
	wait     waitFunc
	now      func() time.Time

	mu      sync.Mutex
	started bool
}

func New(version string, repo Repository, reporter Reporter) *Service {
	return newService(version, repo, reporter, waitForInterval, time.Now)
}

func newService(
	version string,
	repo Repository,
	reporter Reporter,
	wait waitFunc,
	now func() time.Time,
) *Service {
	return &Service{version: version, repo: repo, reporter: reporter, wait: wait, now: now}
}

// Start launches one process-lifetime reporting loop. Calls after the first
// are no-ops, so composition and future reconcilers cannot duplicate the
// heartbeat. Reporting failures never escape this worker or trigger an early
// retry; the next attempt uses the same weekly interval.
func (s *Service) Start(ctx context.Context) {
	if !shouldReportVersion(s.version) {
		return
	}
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return
	}
	s.started = true
	s.mu.Unlock()

	go s.run(ctx)
}

func (s *Service) run(ctx context.Context) {
	for ctx.Err() == nil {
		s.report(ctx)
		if !s.wait(ctx, reportInterval) {
			return
		}
	}
}

func (s *Service) report(ctx context.Context) {
	installationID, err := s.repo.InstallationID()
	if err != nil {
		return
	}

	now := s.now()
	lastAttempt, found, err := s.repo.LastAttempt()
	if err != nil || found && !reportDue(lastAttempt, s.version, now) {
		return
	}
	if err := s.repo.SaveAttempt(Attempt{Version: s.version, At: now}); err != nil {
		return
	}
	_ = s.reporter.ReportVersion(ctx, Report{
		InstallationID: installationID,
		Version:        s.version,
	})
}

func reportDue(lastAttempt Attempt, version string, now time.Time) bool {
	if lastAttempt.Version != version {
		return true
	}
	elapsed := now.Sub(lastAttempt.At)
	return elapsed < 0 || elapsed >= reportInterval
}

func waitForInterval(ctx context.Context, interval time.Duration) bool {
	timer := time.NewTimer(interval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func shouldReportVersion(version string) bool {
	return version != "dev" && !strings.HasPrefix(version, "qa-")
}
