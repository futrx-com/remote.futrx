package runchanges

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/integration/gitcli"
	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
)

type scriptedGit struct {
	head    string
	headErr error
	summary gitcli.DiffSummary
	diffErr error
}

func (g *scriptedGit) Head(context.Context, string) (string, error) {
	return g.head, g.headErr
}

func (g *scriptedGit) DiffSummary(context.Context, string, string) (gitcli.DiffSummary, error) {
	return g.summary, g.diffErr
}

func readRecords(t *testing.T, root string, chatID servicechat.ID) []RunRecord {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(root, "chats", string(chatID), "runs.jsonl"))
	if err != nil {
		t.Fatalf("read runs.jsonl: %v", err)
	}
	var records []RunRecord
	for _, line := range strings.Split(strings.TrimSpace(string(content)), "\n") {
		var record RunRecord
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("unmarshal %q: %v", line, err)
		}
		records = append(records, record)
	}
	return records
}

func TestBeginFinishPersistsChangeset(t *testing.T) {
	root := t.TempDir()
	git := &scriptedGit{
		head: "abc123",
		summary: gitcli.DiffSummary{
			Files:      []string{"a.go", "b.md"},
			Insertions: 10,
			Deletions:  2,
		},
	}
	tracker := NewTracker(root, git)
	scope := tracker.BeginRun(context.Background(), "chat1", "run-1", "codex", "gpt-x", "/repo")
	if scope == nil {
		t.Fatal("BeginRun = nil, want scope")
	}
	scope.Finish(context.Background(), OutcomeCompleted)

	records := readRecords(t, root, "chat1")
	if len(records) != 1 {
		t.Fatalf("records = %d, want 1", len(records))
	}
	record := records[0]
	if record.RunID != "run-1" || record.Provider != "codex" || record.Model != "gpt-x" {
		t.Fatalf("identity = %+v", record)
	}
	if record.Outcome != OutcomeCompleted || record.BaseCommit != "abc123" {
		t.Fatalf("outcome/base = %+v", record)
	}
	if len(record.Files) != 2 || record.Insertions != 10 || record.Deletions != 2 {
		t.Fatalf("changeset = %+v", record)
	}
	if record.StartedAtMs <= 0 || record.EndedAtMs < record.StartedAtMs {
		t.Fatalf("timestamps = %+v", record)
	}
}

func TestBeginRunSkipsNonRepositories(t *testing.T) {
	tracker := NewTracker(t.TempDir(), &scriptedGit{headErr: errors.New("not a repo")})
	if scope := tracker.BeginRun(context.Background(), "chat1", "run-1", "codex", "m", "/repo"); scope != nil {
		t.Fatal("BeginRun = scope, want nil for unreadable HEAD")
	}
	// Nil tracker and nil scope are both safe no-ops.
	var nilTracker *Tracker
	if scope := nilTracker.BeginRun(context.Background(), "chat1", "run-1", "codex", "m", "/repo"); scope != nil {
		t.Fatal("nil BeginRun = scope, want nil")
	}
	var nilScope *Scope
	nilScope.Finish(context.Background(), OutcomeCompleted)
	emptyTracker := NewTracker(t.TempDir(), &scriptedGit{})
	if scope := emptyTracker.BeginRun(context.Background(), "chat1", "run-1", "codex", "m", ""); scope != nil {
		t.Fatal("BeginRun with empty path = scope, want nil")
	}
}

func TestFinishSurvivesDiffErrors(t *testing.T) {
	root := t.TempDir()
	git := &scriptedGit{head: "abc123", diffErr: errors.New("diff failed")}
	tracker := NewTracker(root, git)
	scope := tracker.BeginRun(context.Background(), "chat1", "run-1", "codex", "m", "/repo")
	scope.Finish(context.Background(), OutcomeFailed)

	records := readRecords(t, root, "chat1")
	if len(records) != 1 || records[0].Outcome != OutcomeFailed {
		t.Fatalf("records = %+v", records)
	}
	if records[0].Files == nil {
		t.Fatal("Files = nil, want empty array for JSON consumers")
	}
}

func TestOutcomeFor(t *testing.T) {
	isFailed := func(err error) bool { return errors.Is(err, errRunFailed) }
	if got := OutcomeFor(context.Canceled, nil, isFailed); got != OutcomeInterrupted {
		t.Fatalf("canceled ctx = %q, want interrupted", got)
	}
	if got := OutcomeFor(nil, nil, isFailed); got != OutcomeCompleted {
		t.Fatalf("nil err = %q, want completed", got)
	}
	if got := OutcomeFor(nil, errRunFailed, isFailed); got != OutcomeFailed {
		t.Fatalf("run failure = %q, want failed", got)
	}
	if got := OutcomeFor(nil, errors.New("boom"), isFailed); got != OutcomeError {
		t.Fatalf("other err = %q, want error", got)
	}
}

var errRunFailed = errors.New("run failed")
