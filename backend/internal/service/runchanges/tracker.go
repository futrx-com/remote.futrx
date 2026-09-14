// Package runchanges records per-agent-run workspace changesets.
//
// When an agent run starts inside a git repository, the tracker captures the
// HEAD baseline; when the run finishes it diffs the worktree against that
// baseline and appends one JSON record per run to
// <dataDir>/chats/<chatID>/runs.jsonl. That answers "what exactly did this
// specific agent run change" without disturbing the repository (read-only git
// use; no staging, no commits, no checkouts).
package runchanges

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/integration/gitcli"
	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
)

// Outcome classifies how an agent run ended.
type Outcome string

const (
	OutcomeCompleted   Outcome = "completed"
	OutcomeFailed      Outcome = "failed"
	OutcomeInterrupted Outcome = "interrupted"
	OutcomeError       Outcome = "error"
)

// OutcomeFor maps run termination to an Outcome: a canceled context means the
// user (or scheduler) interrupted the run even when the provider returned nil.
func OutcomeFor(ctxErr, runErr error, runFailed func(error) bool) Outcome {
	if ctxErr != nil {
		return OutcomeInterrupted
	}
	if runErr == nil {
		return OutcomeCompleted
	}
	if runFailed(runErr) {
		return OutcomeFailed
	}
	return OutcomeError
}

// RunRecord is one appended line of runs.jsonl. File paths are repository
// relative, so records carry no host layout.
type RunRecord struct {
	RunID       string   `json:"run_id"`
	Provider    string   `json:"provider"`
	Model       string   `json:"model"`
	StartedAtMs int64    `json:"started_at_ms"`
	EndedAtMs   int64    `json:"ended_at_ms"`
	Outcome     Outcome  `json:"outcome"`
	BaseCommit  string   `json:"base_commit"`
	Files       []string `json:"files"`
	Insertions  int64    `json:"insertions"`
	Deletions   int64    `json:"deletions"`
}

// GitClient is the read-only git surface the tracker needs.
type GitClient interface {
	Head(ctx context.Context, repositoryPath string) (string, error)
	DiffSummary(ctx context.Context, repositoryPath, base string) (gitcli.DiffSummary, error)
}

// Tracker captures baselines and persists per-run changeset records.
// A nil *Tracker or nil *Scope is always safe to use (tracking disabled).
type Tracker struct {
	root  string
	git   GitClient
	mu    sync.Mutex
	locks map[servicechat.ID]*sync.Mutex
}

// NewTracker builds a tracker persisting under <dataDir>/chats/<chatID>/runs.jsonl.
func NewTracker(dataDir string, git GitClient) *Tracker {
	return &Tracker{root: dataDir, git: git, locks: map[servicechat.ID]*sync.Mutex{}}
}

func (t *Tracker) lock(id servicechat.ID) *sync.Mutex {
	t.mu.Lock()
	defer t.mu.Unlock()
	if lock, ok := t.locks[id]; ok {
		return lock
	}
	lock := &sync.Mutex{}
	t.locks[id] = lock
	return lock
}

// Scope carries one run's baseline; Finish is safe on a nil Scope.
type Scope struct {
	tracker   *Tracker
	chatID    servicechat.ID
	runID     string
	provider  string
	model     string
	repoPath  string
	base      string
	startedMs int64
}

// BeginRun captures the HEAD baseline. It returns nil when there is nothing
// to track: empty repo path, missing git client, or HEAD unreadable (not a
// repository, empty repository, or git unavailable).
func (t *Tracker) BeginRun(
	ctx context.Context,
	chatID servicechat.ID,
	runID, provider, model, repoPath string,
) *Scope {
	if t == nil || t.git == nil || repoPath == "" {
		return nil
	}
	base, err := t.git.Head(ctx, repoPath)
	if err != nil || base == "" {
		return nil
	}
	return &Scope{
		tracker:   t,
		chatID:    chatID,
		runID:     runID,
		provider:  provider,
		model:     model,
		repoPath:  repoPath,
		base:      base,
		startedMs: time.Now().UnixMilli(),
	}
}

// Finish diffs the worktree against the baseline and appends the record.
// Errors are swallowed: change tracking must never break a chat turn.
func (s *Scope) Finish(ctx context.Context, outcome Outcome) {
	if s == nil {
		return
	}
	record := RunRecord{
		RunID:       s.runID,
		Provider:    s.provider,
		Model:       s.model,
		StartedAtMs: s.startedMs,
		EndedAtMs:   time.Now().UnixMilli(),
		Outcome:     outcome,
		BaseCommit:  s.base,
	}
	if summary, err := s.tracker.git.DiffSummary(ctx, s.repoPath, s.base); err == nil {
		record.Files = summary.Files
		record.Insertions = summary.Insertions
		record.Deletions = summary.Deletions
	}
	if record.Files == nil {
		record.Files = []string{}
	}
	s.tracker.append(s.chatID, record)
}

func (t *Tracker) append(chatID servicechat.ID, record RunRecord) {
	line, err := json.Marshal(record)
	if err != nil {
		return
	}
	line = append(line, '\n')
	lock := t.lock(chatID)
	lock.Lock()
	defer lock.Unlock()
	dir := filepath.Join(t.root, "chats", string(chatID))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	file, err := os.OpenFile(filepath.Join(dir, "runs.jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer file.Close()
	_, _ = file.Write(line)
}
