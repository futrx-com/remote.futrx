// Package selfupdate checks the installed checkout's origin for newer
// release tags and applies updates by running infra/update.sh detached from
// the service unit. Run state lives on disk under DATA_DIR/self-update/ so
// it survives the backend restart that every successful update performs.
package selfupdate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/service/audit"
)

// HostClient is implemented by integration/updatecli.
type HostClient interface {
	ListRemoteTags(ctx context.Context, installDir string) ([]string, error)
	StartUpdater(installDir, tag, logPath, donePath string) (int, error)
	ProcessAlive(pid int) bool
}

var (
	ErrUpdateInProgress = errors.New("an update is already running")
	ErrNoReleaseTag     = errors.New("no release tags found on origin")
	ErrUnknownTag       = errors.New("tag does not exist on origin")
)

const (
	stateDirName = "self-update"
	logTailBytes = 16 * 1024
	runFileMode  = 0o600
)

type Service struct {
	currentVersion string
	installDir     string
	stateDir       string
	host           HostClient
	audit          audit.Recorder

	mu        sync.Mutex
	lastCheck *CheckResult
}

// Option configures optional Service collaborators.
type Option func(*Service)

// WithAudit records who triggered an application update, and toward which tag.
func WithAudit(recorder audit.Recorder) Option {
	return func(s *Service) { s.audit = audit.RecorderOrNop(recorder) }
}

func New(currentVersion, installDir, dataDir string, host HostClient, options ...Option) *Service {
	service := &Service{
		currentVersion: currentVersion,
		installDir:     installDir,
		stateDir:       filepath.Join(dataDir, stateDirName),
		host:           host,
		audit:          audit.Nop{},
	}
	for _, option := range options {
		if option != nil {
			option(service)
		}
	}
	return service
}

// CheckResult is the outcome of one tag lookup against origin.
type CheckResult struct {
	CheckedAt       int64  `json:"checkedAt"`
	LatestTag       string `json:"latestTag,omitempty"`
	UpdateAvailable bool   `json:"updateAvailable"`
	Error           string `json:"error,omitempty"`
}

// RunStatus describes the most recent apply run, reconstructed from disk so
// it stays accurate across the restart the update itself triggers.
type RunStatus struct {
	State      string `json:"state"` // running | succeeded | failed
	Target     string `json:"target"`
	StartedAt  int64  `json:"startedAt"`
	StartedBy  string `json:"startedBy,omitempty"`
	FinishedAt int64  `json:"finishedAt,omitempty"`
	ExitCode   *int   `json:"exitCode,omitempty"`
	Log        string `json:"log,omitempty"`
}

type Status struct {
	CurrentVersion string       `json:"currentVersion"`
	LastCheck      *CheckResult `json:"lastCheck,omitempty"`
	Run            *RunStatus   `json:"run,omitempty"`
}

type runRecord struct {
	Target    string `json:"target"`
	StartedAt int64  `json:"startedAt"`
	StartedBy string `json:"startedBy"`
	PID       int    `json:"pid"`
}

type doneRecord struct {
	ExitCode   int   `json:"exitCode"`
	FinishedAt int64 `json:"finishedAt"`
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
		Run:            s.runStatus(),
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
		}
	}
	s.mu.Lock()
	s.lastCheck = &result
	s.mu.Unlock()
	return s.Status(ctx)
}

// Apply starts infra/update.sh toward the given tag (or the newest release
// tag when tag is empty). Single-flight: a second call while a run is alive
// returns ErrUpdateInProgress.
func (s *Service) Apply(ctx context.Context, startedBy, tag string) (Status, error) {
	status, resolvedTag, err := s.apply(ctx, startedBy, tag)
	if s.audit != nil {
		entry := audit.Result(
			audit.ActionSelfUpdateTrigger,
			audit.Target{Type: audit.TargetServer, ID: "self-update", Name: resolvedTag},
			audit.Meta{"tag": resolvedTag, "from": s.currentVersion},
			err,
		)
		if startedBy != "" {
			entry.Actor = audit.Actor{Email: audit.NormalizeActorEmail(startedBy), IsAdmin: true}
		}
		s.audit.Record(ctx, entry)
	}
	return status, err
}

// apply also returns the tag it resolved, so the audit entry names the target
// even when the caller left it blank and the newest release was chosen.
func (s *Service) apply(ctx context.Context, startedBy, tag string) (Status, string, error) {
	tags, err := s.host.ListRemoteTags(ctx, s.installDir)
	if err != nil {
		return s.Status(ctx), tag, fmt.Errorf("list origin tags: %w", err)
	}
	if tag == "" {
		if tag, _ = latestReleaseTag(tags); tag == "" {
			return s.Status(ctx), tag, ErrNoReleaseTag
		}
	} else if !containsTag(tags, tag) {
		return s.Status(ctx), tag, fmt.Errorf("%w: %s", ErrUnknownTag, tag)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if run := s.runStatus(); run != nil && run.State == "running" {
		return s.statusLocked(), tag, ErrUpdateInProgress
	}
	if err := os.MkdirAll(s.stateDir, 0o700); err != nil {
		return s.statusLocked(), tag, err
	}
	// A fresh run replaces the previous run's records.
	if err := os.Remove(s.donePath()); err != nil && !os.IsNotExist(err) {
		return s.statusLocked(), tag, err
	}
	if err := os.WriteFile(s.logPath(), nil, runFileMode); err != nil {
		return s.statusLocked(), tag, err
	}
	pid, err := s.host.StartUpdater(s.installDir, tag, s.logPath(), s.donePath())
	if err != nil {
		return s.statusLocked(), tag, fmt.Errorf("start updater: %w", err)
	}
	record := runRecord{Target: tag, StartedAt: time.Now().Unix(), StartedBy: startedBy, PID: pid}
	if err := writeJSONFile(s.runPath(), record); err != nil {
		return s.statusLocked(), tag, err
	}
	return s.statusLocked(), tag, nil
}

func (s *Service) statusLocked() Status {
	return Status{
		CurrentVersion: s.currentVersion,
		LastCheck:      s.lastCheck,
		Run:            s.runStatus(),
	}
}

func (s *Service) runPath() string  { return filepath.Join(s.stateDir, "run.json") }
func (s *Service) donePath() string { return filepath.Join(s.stateDir, "done.json") }
func (s *Service) logPath() string  { return filepath.Join(s.stateDir, "run.log") }

// runStatus reconstructs the last run from disk: the done marker wins, a
// live PID means running, and a dead PID without a marker means the run
// crashed before it could report.
func (s *Service) runStatus() *RunStatus {
	var record runRecord
	if err := readJSONFile(s.runPath(), &record); err != nil {
		return nil
	}
	status := &RunStatus{
		State:     "running",
		Target:    record.Target,
		StartedAt: record.StartedAt,
		StartedBy: record.StartedBy,
		Log:       tailFile(s.logPath(), logTailBytes),
	}
	var done doneRecord
	switch err := readJSONFile(s.donePath(), &done); {
	case err == nil:
		status.FinishedAt = done.FinishedAt
		status.ExitCode = &done.ExitCode
		if done.ExitCode == 0 {
			status.State = "succeeded"
		} else {
			status.State = "failed"
		}
	case !s.host.ProcessAlive(record.PID):
		status.State = "failed"
		status.Log += "\n(updater process exited without reporting a result)"
	}
	return status
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

func readJSONFile(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

func writeJSONFile(path string, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, runFileMode)
}

// tailFile returns up to the last max bytes of the file at path.
func tailFile(path string, max int64) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return ""
	}
	if size := info.Size(); size > max {
		if _, err := f.Seek(size-max, io.SeekStart); err != nil {
			return ""
		}
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return ""
	}
	return string(data)
}
