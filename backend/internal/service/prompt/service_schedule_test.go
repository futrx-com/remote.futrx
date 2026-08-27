package prompt

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
	"github.com/futrx-com/remote.futrx.com/internal/service/runhub"
	"github.com/futrx-com/remote.futrx.com/internal/stores/filechat"
)

type schedulePromptProvider struct {
	mu       sync.Mutex
	requests []agent.RunRequest
	started  chan struct{}
	release  <-chan struct{}
	once     sync.Once
	output   []string
}

func (p *schedulePromptProvider) ID() agent.ProviderID { return agent.ProviderCodex }

func (p *schedulePromptProvider) Parser(agent.RunRequest) agent.LineParser { return nil }

func (p *schedulePromptProvider) Capabilities(context.Context, agent.CapabilityRequest) (agent.Capabilities, error) {
	return agent.Capabilities{Provider: agent.ProviderCodex}, nil
}

func (p *schedulePromptProvider) Run(
	ctx context.Context,
	req agent.RunRequest,
	emit func(agent.Event),
) error {
	p.mu.Lock()
	p.requests = append(p.requests, req)
	p.mu.Unlock()
	if p.started != nil {
		p.once.Do(func() { close(p.started) })
	}
	if p.release != nil {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-p.release:
		}
	}
	for _, text := range p.output {
		emit(agent.Event{Type: agent.EventAssistantTextDelta, Text: text})
	}
	emit(agent.Event{Type: agent.EventRunCompleted})
	return nil
}

func (p *schedulePromptProvider) request(t *testing.T, index int) agent.RunRequest {
	t.Helper()
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.requests) <= index {
		t.Fatalf("provider requests = %d, want index %d", len(p.requests), index)
	}
	return p.requests[index]
}

type recordingScheduleIssuer struct {
	mu       sync.Mutex
	requests []ScheduleToolRequest
	access   ScheduleToolAccess
}

func (i *recordingScheduleIssuer) IssueScheduleTool(
	_ context.Context,
	req ScheduleToolRequest,
) (ScheduleToolAccess, error) {
	i.mu.Lock()
	i.requests = append(i.requests, req)
	i.mu.Unlock()
	return i.access, nil
}

func (i *recordingScheduleIssuer) request(t *testing.T, index int) ScheduleToolRequest {
	t.Helper()
	i.mu.Lock()
	defer i.mu.Unlock()
	if len(i.requests) <= index {
		t.Fatalf("issuer requests = %d, want index %d", len(i.requests), index)
	}
	return i.requests[index]
}

func TestStartWithScheduledTasksSkillIssuesManageCapabilityAndReturnsOutput(t *testing.T) {
	ctx := context.Background()
	store, err := filechat.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	meta, err := store.Create(ctx, servicechat.Meta{
		ID:        "aabbcc11",
		Title:     "watch deploy",
		Provider:  servicechat.ProviderCodex,
		Cwd:       t.TempDir(),
		ProjectID: "project-1",
		SelectedSkills: []servicechat.SkillRef{{
			Name:     "Scheduled Tasks",
			Command:  scheduledTasksSkillName,
			Provider: servicechat.ProviderCodex,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}

	provider := &schedulePromptProvider{output: []string{"deployment healthy\n", "TASK_COMPLETE"}}
	registry := agent.NewRegistry()
	if err := registry.Register(provider); err != nil {
		t.Fatal(err)
	}
	var revoked atomic.Bool
	issuer := &recordingScheduleIssuer{access: ScheduleToolAccess{
		APIURL: "https://remote.test/agent-api/schedules",
		Token:  "manage-token",
		Revoke: func() {
			revoked.Store(true)
		},
	}}
	service := New(
		store,
		nil,
		nil,
		runhub.New(store),
		registry,
		WithScheduleToolIssuer(issuer),
		WithAgentPolicy(codexTestAgentPolicy()),
	)

	actor := Actor{Email: "owner@example.com", IsAdmin: true}
	handle, err := service.Start(StartInput{
		ChatID: meta.ID,
		Prompt: "keep watching",
		Actor:  actor,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if handle.ID == 0 {
		t.Fatal("run handle ID is zero")
	}
	result := awaitScheduleRun(t, handle)
	if result.Err != nil {
		t.Fatalf("run result error = %v", result.Err)
	}
	if result.Output != "deployment healthy\nTASK_COMPLETE" {
		t.Fatalf("run output = %q", result.Output)
	}
	if !revoked.Load() {
		t.Fatal("schedule capability was not revoked after the run")
	}

	issued := issuer.request(t, 0)
	if issued.Actor != actor || issued.ChatID != meta.ID ||
		string(issued.ProjectID) != "project-1" || issued.ScheduledTaskID != "" {
		t.Fatalf("issuer request = %#v", issued)
	}
	request := provider.request(t, 0)
	if !request.EnableScheduleTools {
		t.Fatal("scheduled-tasks skill did not enable schedule tools")
	}
	if request.RuntimeEnv["REMOTE_SCHEDULE_API"] != issuer.access.APIURL ||
		request.RuntimeEnv["REMOTE_SCHEDULE_GRANT"] != issuer.access.Token {
		t.Fatalf("runtime env = %#v", request.RuntimeEnv)
	}
	if !strings.Contains(request.Prompt, "$"+scheduledTasksSkillName) {
		t.Fatalf("provider prompt missing selected skill trigger: %q", request.Prompt)
	}
}

func TestStartScheduledTaskRequestsCompletionOnlyCapability(t *testing.T) {
	ctx := context.Background()
	store, err := filechat.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	meta, err := store.Create(ctx, servicechat.Meta{
		ID:        "aabbcc22",
		Title:     "scheduled run",
		Provider:  servicechat.ProviderCodex,
		Cwd:       t.TempDir(),
		ProjectID: "project-2",
	})
	if err != nil {
		t.Fatal(err)
	}

	provider := &schedulePromptProvider{}
	registry := agent.NewRegistry()
	if err := registry.Register(provider); err != nil {
		t.Fatal(err)
	}
	issuer := &recordingScheduleIssuer{access: ScheduleToolAccess{
		APIURL: "https://remote.test/agent-api/schedules",
		Token:  "complete-only-token",
	}}
	service := New(
		store,
		nil,
		nil,
		runhub.New(store),
		registry,
		WithScheduleToolIssuer(issuer),
		WithAgentPolicy(codexTestAgentPolicy()),
	)

	const scheduledTaskID = "task-123"
	const scheduledRunID = "run-456"
	handle, err := service.Start(StartInput{
		ChatID:          meta.ID,
		Prompt:          `[Scheduled task "watch deploy", fire 3/12] Continue the standing task.`,
		Actor:           Actor{Email: "owner@example.com"},
		ScheduledTaskID: scheduledTaskID,
		ScheduledRunID:  scheduledRunID,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result := awaitScheduleRun(t, handle); result.Err != nil {
		t.Fatalf("run result error = %v", result.Err)
	}

	issued := issuer.request(t, 0)
	if issued.ScheduledTaskID != scheduledTaskID {
		t.Fatalf("scheduled task ID = %q, want %q", issued.ScheduledTaskID, scheduledTaskID)
	}
	if issued.ScheduledRunID != scheduledRunID {
		t.Fatalf("scheduled run ID = %q, want %q", issued.ScheduledRunID, scheduledRunID)
	}
	request := provider.request(t, 0)
	if !request.EnableScheduleTools {
		t.Fatal("scheduled run did not enable completion tooling")
	}
	if request.RuntimeEnv["REMOTE_SCHEDULE_GRANT"] != "complete-only-token" {
		t.Fatalf("runtime grant = %q", request.RuntimeEnv["REMOTE_SCHEDULE_GRANT"])
	}
	if !strings.Contains(request.Prompt, "$"+scheduledTasksSkillName) {
		t.Fatalf("scheduled run prompt missing required skill trigger: %q", request.Prompt)
	}
}

func TestStartReturnsBusyWhilePriorRunIsActive(t *testing.T) {
	ctx := context.Background()
	store, err := filechat.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	meta, err := store.Create(ctx, servicechat.Meta{
		ID:       "aabbcc33",
		Title:    "busy",
		Provider: servicechat.ProviderCodex,
		Cwd:      t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}

	release := make(chan struct{})
	provider := &schedulePromptProvider{
		started: make(chan struct{}),
		release: release,
	}
	registry := agent.NewRegistry()
	if err := registry.Register(provider); err != nil {
		t.Fatal(err)
	}
	service := New(store, nil, nil, runhub.New(store), registry)

	first, err := service.Start(StartInput{ChatID: meta.ID, Prompt: "first"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-provider.started:
	case <-time.After(2 * time.Second):
		t.Fatal("provider did not start")
	}

	_, err = service.Start(StartInput{ChatID: meta.ID, Prompt: "second"}, nil)
	if !errors.Is(err, ErrPromptAlreadyRunning) {
		t.Fatalf("second Start error = %v, want %v", err, ErrPromptAlreadyRunning)
	}
	close(release)
	if result := awaitScheduleRun(t, first); result.Err != nil {
		t.Fatalf("first run result error = %v", result.Err)
	}
}

func TestStartCancelsRunWithParentContext(t *testing.T) {
	t.Parallel()
	store, err := filechat.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	meta, err := store.Create(context.Background(), servicechat.Meta{
		ID:       "aabbcc44",
		Title:    "scheduled cancellation",
		Provider: servicechat.ProviderCodex,
		Cwd:      t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}

	provider := &schedulePromptProvider{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	registry := agent.NewRegistry()
	if err := registry.Register(provider); err != nil {
		t.Fatal(err)
	}
	service := New(store, nil, nil, runhub.New(store), registry)
	parentCtx, cancel := context.WithCancel(context.Background())
	handle, err := service.Start(StartInput{
		ChatID:        meta.ID,
		Prompt:        "continue",
		ParentContext: parentCtx,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-provider.started:
	case <-time.After(2 * time.Second):
		t.Fatal("provider did not start")
	}

	cancel()
	result := awaitScheduleRun(t, handle)
	if !errors.Is(result.Err, context.Canceled) {
		t.Fatalf("run error = %v, want context canceled", result.Err)
	}
}

func awaitScheduleRun(t *testing.T, handle RunHandle) RunResult {
	t.Helper()
	select {
	case result, ok := <-handle.Done:
		if !ok {
			t.Fatal("run result channel closed without a result")
		}
		return result
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for run result")
		return RunResult{}
	}
}
