// Package selfupdate checks the installed checkout's origin for newer
// release tags and applies them with either application deployment or full
// infrastructure convergence detached from the service unit. Run state lives
// on disk under DATA_DIR/self-update/ so it survives the backend restart that
// every successful update performs.
package selfupdate

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	ErrUpdateInProgress = errors.New("an update is already running")
	ErrNoReleaseTag     = errors.New("no release tags found on origin")
	ErrUnknownTag       = errors.New("tag does not exist on origin")
)

const lifecycleReconcileInterval = time.Second

type Service struct {
	currentVersion string
	installDir     string
	host           HostClient
	lifecycle      UpdateLifecyclePublisher
	runs           runState

	mu          sync.Mutex
	lastCheck   *CheckResult
	launching   bool
	reconciling bool
	dispatching bool
}

func New(
	currentVersion, installDir, dataDir string,
	host HostClient,
	lifecycle UpdateLifecyclePublisher,
) *Service {
	return &Service{
		currentVersion: currentVersion,
		installDir:     installDir,
		host:           host,
		lifecycle:      lifecycle,
		runs:           newRunState(dataDir),
	}
}

// Status reports the running version, the last check result, and the most
// recent apply run.
func (s *Service) Status(context.Context) Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.statusLocked()
}

// Check queries origin for release tags and records whether one is newer
// than the running version.
func (s *Service) Check(ctx context.Context) Status {
	result := CheckResult{CheckedAt: time.Now().Unix()}
	tags, err := s.host.ListRemoteTags(ctx, s.installDir)
	if err != nil {
		result.Error = err.Error()
	} else {
		latest, latestSegments := latestReleaseTag(tags)
		result.LatestTag = latest
		if current, ok := parseReleaseTag(describeBase(s.currentVersion)); ok && latest != "" {
			result.UpdateAvailable = compareVersions(latestSegments, current) > 0
			if result.UpdateAvailable {
				result.UpdateKind = classifyUpdate(s.currentVersion, latest)
			}
		}
	}
	s.mu.Lock()
	s.lastCheck = &result
	s.mu.Unlock()
	return s.Status(ctx)
}

// Apply starts the safe deployment path toward the given tag (or the newest
// release tag when tag is empty). Single-flight: a second call while a run is
// alive returns ErrUpdateInProgress.
func (s *Service) Apply(ctx context.Context, startedBy, tag string) (Status, error) {
	tags, err := s.host.ListRemoteTags(ctx, s.installDir)
	if err != nil {
		return s.Status(ctx), fmt.Errorf("list origin tags: %w", err)
	}
	if tag == "" {
		if tag, _ = latestReleaseTag(tags); tag == "" {
			return s.Status(ctx), ErrNoReleaseTag
		}
	} else if !containsTag(tags, tag) {
		return s.Status(ctx), fmt.Errorf("%w: %s", ErrUnknownTag, tag)
	}

	status, err := s.startUpdate(ctx, startedBy, tag)
	if err != nil {
		return status, err
	}
	return status, nil
}

func (s *Service) startUpdate(ctx context.Context, startedBy, tag string) (Status, error) {
	s.mu.Lock()
	if s.launching {
		status := s.statusLocked()
		s.mu.Unlock()
		return status, ErrUpdateInProgress
	}
	// Capture the previous run BEFORE reset so we can reuse its classification
	// on retry; reset clears run.json as part of the fresh-slate contract.
	prevRun := s.runs.status(s.host.ProcessAlive)
	if prevRun != nil && prevRun.State == RunStateRunning {
		status := s.statusLocked()
		s.mu.Unlock()
		return status, ErrUpdateInProgress
	}
	if err := s.runs.reset(); err != nil {
		status := s.statusLocked()
		s.mu.Unlock()
		return status, err
	}
	// A failed infrastructure update may have already replaced the binary,
	// so classifyUpdate against currentVersion would collapse to an
	// application-only deploy and skip the host convergence that actually
	// failed. Fall back to the previous failed run's kind when retrying
	// toward the same target.
	kind := classifyUpdate(s.currentVersion, tag)
	if prevRun != nil && prevRun.State == RunStateFailed && prevRun.Target == tag && prevRun.UpdateKind != "" {
		kind = prevRun.UpdateKind
	}
	message := "Preparing the infrastructure update"
	if kind == UpdateKindApplication {
		message = "Preparing the application update"
	}
	if err := s.runs.writeProgress(Progress{
		Phase: "preparing", Message: message, UpdatedAt: time.Now().Unix(),
	}); err != nil {
		status := s.statusLocked()
		s.mu.Unlock()
		return status, err
	}
	s.launching = true
	s.mu.Unlock()

	// Started is deliberately synchronous and precedes the detached process, so
	// subscribers observe the transition before that process can replace this
	// backend. Notifications cannot veto the launch.
	s.lifecycle.PublishUpdateStarted(ctx, tag, string(kind), startedBy)
	pid, err := s.host.StartUpdater(s.runs.launch(s.installDir, tag, kind))

	s.mu.Lock()
	s.launching = false
	if err != nil {
		// The new run never started; clear the half-written record so
		// Status() does not report a stale run with the next attempt's
		// target and an empty log. Kind preservation for the next call
		// is best-effort: only the in-memory prevRun survives reset().
		s.runs.removeProgress()
		s.runs.removeRecord()
		status := s.statusLocked()
		s.mu.Unlock()
		s.lifecycle.PublishUpdateFailed(ctx, tag, string(kind), startedBy)
		return status, fmt.Errorf("start updater: %w", err)
	}
	record := runRecord{
		Target: tag, UpdateKind: kind, StartedAt: time.Now().Unix(), StartedBy: startedBy, PID: pid,
	}
	if err := s.runs.writeRecord(record); err != nil {
		status := s.statusLocked()
		s.mu.Unlock()
		return status, err
	}
	status := s.statusLocked()
	s.mu.Unlock()
	return status, nil
}

// StartLifecycleReconciler delivers terminal update events from the durable
// run state. A successful updater restarts the backend before it writes its
// done marker, so the replacement process must resume this reconciliation;
// an in-memory callback owned by the process that launched the updater cannot
// observe completion reliably.
func (s *Service) StartLifecycleReconciler(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := s.reconcileLifecycle(ctx); err != nil {
		return err
	}

	s.mu.Lock()
	if s.reconciling {
		s.mu.Unlock()
		return nil
	}
	s.reconciling = true
	s.mu.Unlock()

	go func() {
		ticker := time.NewTicker(lifecycleReconcileInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// A transient read/write failure is retried on the next tick. The
				// synchronous first pass above is returned to startup for logging.
				_ = s.reconcileLifecycle(ctx)
			}
		}
	}()
	return nil
}

func (s *Service) reconcileLifecycle(ctx context.Context) error {
	s.mu.Lock()
	if s.dispatching {
		s.mu.Unlock()
		return nil
	}

	record, err := s.runs.readRecord()
	if errors.Is(err, os.ErrNotExist) {
		s.mu.Unlock()
		return nil
	}
	if err != nil {
		s.mu.Unlock()
		return fmt.Errorf("read update lifecycle state: %w", err)
	}
	status := s.runs.status(s.host.ProcessAlive)
	if status == nil || status.State == RunStateRunning || record.PublishedTerminalState == status.State {
		s.mu.Unlock()
		return nil
	}
	s.dispatching = true
	s.mu.Unlock()

	switch status.State {
	case RunStateSucceeded:
		s.lifecycle.PublishUpdateSucceeded(ctx, record.Target, string(record.UpdateKind), record.StartedBy)
	case RunStateFailed:
		s.lifecycle.PublishUpdateFailed(ctx, record.Target, string(record.UpdateKind), record.StartedBy)
	default:
		s.mu.Lock()
		s.dispatching = false
		s.mu.Unlock()
		return nil
	}

	// Persist delivery after dispatch. If the process dies between those two
	// operations, the replacement may deliver the event again; subscribers are
	// therefore required to be idempotent. Losing the terminal event would be
	// worse than an occasional duplicate.
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dispatching = false
	current, err := s.runs.readRecord()
	if err != nil {
		return fmt.Errorf("reread update lifecycle state: %w", err)
	}
	// A new Apply may have replaced the completed run while subscribers were
	// executing. Never stamp the previous event onto that new run.
	if current.Target != record.Target || current.StartedAt != record.StartedAt || current.PID != record.PID {
		return nil
	}
	current.PublishedTerminalState = status.State
	if err := s.runs.writeRecord(current); err != nil {
		return fmt.Errorf("record published update lifecycle state: %w", err)
	}
	return nil
}

func (s *Service) statusLocked() Status {
	return Status{
		CurrentVersion: s.currentVersion,
		LastCheck:      s.lastCheck,
		Run:            s.runs.status(s.host.ProcessAlive),
	}
}

// describeBase extracts the release tag a git-describe string is based on:
// "0.1-12-gdb01776" → "0.1", "v0.2" → "v0.2", "dev" → "dev".
func describeBase(describe string) string {
	base, _, _ := strings.Cut(describe, "-")
	return base
}

// parseReleaseTag parses "0.1", "v0.2.3" and similar numeric release tags
// into version segments. Anything else — branch-like names, "dev", bare
// commit hashes — is not a release tag.
func parseReleaseTag(tag string) ([]int, bool) {
	trimmed := strings.TrimPrefix(tag, "v")
	if trimmed == "" {
		return nil, false
	}
	parts := strings.Split(trimmed, ".")
	segments := make([]int, len(parts))
	for i, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 {
			return nil, false
		}
		segments[i] = n
	}
	return segments, true
}

func compareVersions(a, b []int) int {
	for i := 0; i < len(a) || i < len(b); i++ {
		av, bv := 0, 0
		if i < len(a) {
			av = a[i]
		}
		if i < len(b) {
			bv = b[i]
		}
		if av != bv {
			if av > bv {
				return 1
			}
			return -1
		}
	}
	return 0
}

// classifyUpdate is conservative for legacy or malformed versions. Only
// releases with a major, minor, and patch component in the same release line
// can use the application-only deployment path.
func classifyUpdate(currentVersion, targetTag string) UpdateKind {
	current, currentOK := parseReleaseTag(describeBase(currentVersion))
	target, targetOK := parseReleaseTag(targetTag)
	if !currentOK || !targetOK || len(current) < 3 || len(target) < 3 {
		return UpdateKindInfrastructure
	}
	if current[0] == target[0] && current[1] == target[1] {
		return UpdateKindApplication
	}
	return UpdateKindInfrastructure
}

// latestReleaseTag picks the highest version-shaped tag.
func latestReleaseTag(tags []string) (string, []int) {
	var best string
	var bestSegments []int
	for _, tag := range tags {
		segments, ok := parseReleaseTag(tag)
		if !ok {
			continue
		}
		if best == "" || compareVersions(segments, bestSegments) > 0 {
			best, bestSegments = tag, segments
		}
	}
	return best, bestSegments
}

func containsTag(tags []string, tag string) bool {
	for _, t := range tags {
		if t == tag {
			return true
		}
	}
	return false
}
