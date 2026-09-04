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

type Service struct {
	currentVersion string
	installDir     string
	host           HostClient
	runs           runState

	mu        sync.Mutex
	lastCheck *CheckResult
}

func New(currentVersion, installDir, dataDir string, host HostClient) *Service {
	return &Service{
		currentVersion: currentVersion,
		installDir:     installDir,
		host:           host,
		runs:           newRunState(dataDir),
	}
}

// Status reports the running version, the last check result, and the most
// recent apply run.
func (s *Service) Status(context.Context) Status {
	s.mu.Lock()
	check := s.lastCheck
	s.mu.Unlock()
	return Status{
		CurrentVersion: s.currentVersion,
		LastCheck:      check,
		Run:            s.runs.status(s.host.ProcessAlive),
	}
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

	s.mu.Lock()
	defer s.mu.Unlock()
	if run := s.runs.status(s.host.ProcessAlive); run != nil && run.State == "running" {
		return s.statusLocked(), ErrUpdateInProgress
	}
	// A fresh run replaces the previous run's records.
	if err := s.runs.reset(); err != nil {
		return s.statusLocked(), err
	}
	kind := classifyUpdate(s.currentVersion, tag)
	message := "Preparing the infrastructure update"
	if kind == UpdateKindApplication {
		message = "Preparing the application update"
	}
	if err := s.runs.writeProgress(Progress{
		Phase: "preparing", Message: message, UpdatedAt: time.Now().Unix(),
	}); err != nil {
		return s.statusLocked(), err
	}
	pid, err := s.host.StartUpdater(s.runs.launch(s.installDir, tag, kind))
	if err != nil {
		s.runs.removeProgress()
		return s.statusLocked(), fmt.Errorf("start updater: %w", err)
	}
	record := runRecord{
		Target: tag, UpdateKind: kind, StartedAt: time.Now().Unix(), StartedBy: startedBy, PID: pid,
	}
	if err := s.runs.writeRecord(record); err != nil {
		return s.statusLocked(), err
	}
	return s.statusLocked(), nil
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
