package prompt

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
	"github.com/futrx-com/remote.futrx.com/internal/service/runhub"
	serviceusage "github.com/futrx-com/remote.futrx.com/internal/service/usage"
	"github.com/futrx-com/remote.futrx.com/internal/stores/filechat"
)

// usageProvider emits one turn's worth of events, ending in the completion
// event that carries the provider's usage payload.
type usageProvider struct {
	usage json.RawMessage
	fail  bool
}

func (p *usageProvider) ID() agent.ProviderID                     { return agent.ProviderClaude }
func (p *usageProvider) Parser(agent.RunRequest) agent.LineParser { return nil }

// Capabilities satisfies the interface qa added to agent.Provider. This double
// exists to record usage events, so it reports nothing rather than pretending
// to have a capability set.
func (p *usageProvider) Capabilities(context.Context, agent.CapabilityRequest) (agent.Capabilities, error) {
	return agent.Capabilities{}, nil
}

func (p *usageProvider) Run(
	_ context.Context, _ agent.RunRequest, emit func(agent.Event)) error {
	now := time.Now().UnixMilli()
	emit(agent.Event{T: now, Type: agent.EventAssistantTextDelta, Text: "done"})
	if p.fail {
		emit(agent.Event{
			T: now, Type: agent.EventRunFailed, Provider: agent.ProviderClaude,
			Message: "boom", Usage: p.usage,
		})
		return agent.ErrRunFailed
	}
	emit(agent.Event{
		T: now, Type: agent.EventRunCompleted, Provider: agent.ProviderClaude, Usage: p.usage,
	})
	return nil
}

type recordingLedger struct {
	events []serviceusage.RunEvent
}

func (l *recordingLedger) RecordRun(_ context.Context, event serviceusage.RunEvent) {
	l.events = append(l.events, event)
}

// stubScheduleTools satisfies the schedule-tool port so a scheduled turn can
// reach the provider without a real capability registry.
type stubScheduleTools struct{}

func (stubScheduleTools) IssueScheduleTool(
	context.Context,
	ScheduleToolRequest,
) (ScheduleToolAccess, error) {
	return ScheduleToolAccess{APIURL: "http://127.0.0.1/agent-api", Token: "grant"}, nil
}

func newUsagePromptService(
	t *testing.T,
	provider agent.Provider,
	ledger UsageRecorder,
	options ...Option,
) (*Service, servicechat.Repository, servicechat.Meta) {
	t.Helper()
	store, err := filechat.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	meta, err := store.Create(context.Background(), servicechat.Meta{
		ID:        "abcdef123456",
		Title:     "existing",
		Provider:  servicechat.ProviderClaude,
		Model:     "claude-sonnet-4-5",
		ProjectID: "aaaa1111",
		Cwd:       t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	registry := agent.NewRegistry()
	if err := registry.Register(provider); err != nil {
		t.Fatal(err)
	}
	service := New(
		store, nil, nil, runhub.New(store), registry,
		append([]Option{WithUsageRecorder(ledger)}, options...)...,
	)
	return service, store, meta
}

func TestStartRecordsOneLedgerEntryPerCompletedRun(t *testing.T) {
	ledger := &recordingLedger{}
	provider := &usageProvider{usage: json.RawMessage(
		`{"input_tokens":120,"output_tokens":30,"total_cost_usd":0.11,"model":"claude-opus-4-5"}`,
	)}
	service, _, meta := newUsagePromptService(t, provider, ledger)

	handle, err := service.Start(StartInput{
		ChatID:        meta.ID,
		Prompt:        "hello",
		Actor:         Actor{Email: "member@example.com"},
		ParentContext: context.Background(),
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	<-handle.Done

	if len(ledger.events) != 1 {
		t.Fatalf("ledger entries = %d, want 1: %+v", len(ledger.events), ledger.events)
	}
	event := ledger.events[0]
	if event.ChatID != string(meta.ID) || event.ProjectID != "aaaa1111" {
		t.Fatalf("unexpected identity: %+v", event)
	}
	if event.UserEmail != "member@example.com" || event.Provider != string(agent.ProviderClaude) {
		t.Fatalf("unexpected attribution: %+v", event)
	}
	if event.Scheduled {
		t.Fatalf("interactive run marked scheduled: %+v", event)
	}
	if event.RunID == "" {
		t.Fatal("run id must identify the turn")
	}
	if event.At == 0 {
		t.Fatal("run must be timestamped")
	}
	usage, ok := agent.ParseUsage(event.Usage)
	if !ok || usage.InputTokens != 120 || usage.CostUSD == nil || *usage.CostUSD != 0.11 {
		t.Fatalf("usage payload lost: %s", event.Usage)
	}
}

// A failed run's token counts are not persisted in the chat event log, so the
// ledger skips them; otherwise a rebuild could never reproduce the file.
func TestStartDoesNotRecordFailedRuns(t *testing.T) {
	ledger := &recordingLedger{}
	provider := &usageProvider{fail: true, usage: json.RawMessage(`{"input_tokens":9}`)}
	service, _, meta := newUsagePromptService(t, provider, ledger)

	handle, err := service.Start(StartInput{
		ChatID:        meta.ID,
		Prompt:        "hello",
		Actor:         Actor{Email: "member@example.com"},
		ParentContext: context.Background(),
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	<-handle.Done

	if len(ledger.events) != 0 {
		t.Fatalf("failed run was billed: %+v", ledger.events)
	}
}

// A scheduled turn is billed to its owner and flagged so the Usage page can
// separate unattended spend from interactive spend.
func TestStartMarksScheduledRuns(t *testing.T) {
	ledger := &recordingLedger{}
	provider := &usageProvider{usage: json.RawMessage(`{"input_tokens":5,"output_tokens":1}`)}
	service, _, meta := newUsagePromptService(
		t, provider, ledger, WithScheduleToolIssuer(stubScheduleTools{}),
	)

	handle, err := service.Start(StartInput{
		ChatID:          meta.ID,
		Prompt:          "nightly report",
		Actor:           Actor{Email: "owner@example.com"},
		ScheduledTaskID: "task-1",
		ScheduledRunID:  "run-1",
		ParentContext:   context.Background(),
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	<-handle.Done

	if len(ledger.events) != 1 {
		t.Fatalf("ledger entries = %d, want 1: %+v", len(ledger.events), ledger.events)
	}
	if !ledger.events[0].Scheduled || ledger.events[0].UserEmail != "owner@example.com" {
		t.Fatalf("unexpected scheduled entry: %+v", ledger.events[0])
	}
}

func TestStartWithoutLedgerStillRuns(t *testing.T) {
	provider := &usageProvider{usage: json.RawMessage(`{"input_tokens":1}`)}
	service, store, meta := newUsagePromptService(t, provider, nil)

	handle, err := service.Start(StartInput{
		ChatID:        meta.ID,
		Prompt:        "hello",
		ParentContext: context.Background(),
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	<-handle.Done

	events, err := store.ReadEvents(context.Background(), meta.ID)
	if err != nil {
		t.Fatal(err)
	}
	var completed bool
	for _, event := range events {
		if event.Type == "complete" {
			completed = true
		}
	}
	if !completed {
		t.Fatalf("expected the turn to complete without a ledger: %+v", events)
	}
}
