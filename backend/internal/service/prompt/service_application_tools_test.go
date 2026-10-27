package prompt

import (
	"context"
	"errors"
	"fmt"
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

type applicationPromptProvider struct {
	mu       sync.Mutex
	requests []agent.RunRequest
	started  chan struct{}
	release  <-chan struct{}
	once     sync.Once
	output   []string
}

func (p *applicationPromptProvider) ID() agent.ProviderID { return agent.ProviderCodex }

func (p *applicationPromptProvider) Parser(agent.RunRequest) agent.LineParser { return nil }

func (p *applicationPromptProvider) Capabilities(context.Context, agent.CapabilityRequest) (agent.Capabilities, error) {
	return agent.Capabilities{Provider: agent.ProviderCodex}, nil
}

func (p *applicationPromptProvider) Run(
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

func (p *applicationPromptProvider) request(t *testing.T, index int) agent.RunRequest {
	t.Helper()
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.requests) <= index {
		t.Fatalf("provider requests = %d, want index %d", len(p.requests), index)
	}
	return p.requests[index]
}

type recordingApplicationIssuer struct {
	mu       sync.Mutex
	requests []ApplicationToolRequest
	access   ApplicationToolAccess
}

func (i *recordingApplicationIssuer) IssueApplicationTools(
	_ context.Context,
	req ApplicationToolRequest,
) (ApplicationToolAccess, error) {
	i.mu.Lock()
	i.requests = append(i.requests, req)
	i.mu.Unlock()
	return i.access, nil
}

func (i *recordingApplicationIssuer) request(t *testing.T, index int) ApplicationToolRequest {
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
		AccountID: "work-account",
		Cwd:       t.TempDir(),
		ProjectID: "project-1",
		SelectedSkills: []servicechat.SkillRef{{
			Name:     "Scheduled Tasks",
			Command:  "example-app",
			Provider: servicechat.ProviderCodex,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}

	provider := &applicationPromptProvider{output: []string{"deployment healthy\n", "TASK_COMPLETE"}}
	registry := agent.NewRegistry()
	if err := registry.Register(provider); err != nil {
		t.Fatal(err)
	}
	var revoked atomic.Bool
	issuer := &recordingApplicationIssuer{access: ApplicationToolAccess{
		Env: map[string]string{"REMOTE_APPLICATION_API": "https://remote.test/agent-api/schedules", "REMOTE_APPLICATION_GRANT": "manage-token"}, Skills: []servicechat.SkillRef{{Name: "Example App", Command: "example-app", Source: "remote"}},
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
		WithApplicationToolIssuer(issuer),
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
		string(issued.ProjectID) != "project-1" || issued.ApplicationInstanceID != "" {
		t.Fatalf("issuer request = %#v", issued)
	}
	request := provider.request(t, 0)
	if request.AccountID != "work-account" {
		t.Fatalf("run account = %q, want work-account", request.AccountID)
	}
	if len(request.RuntimeEnv) == 0 {
		t.Fatal("scheduled-tasks skill did not enable schedule tools")
	}
	if request.RuntimeEnv["REMOTE_APPLICATION_API"] != issuer.access.Env["REMOTE_APPLICATION_API"] ||
		request.RuntimeEnv["REMOTE_APPLICATION_GRANT"] != issuer.access.Env["REMOTE_APPLICATION_GRANT"] {
		t.Fatalf("runtime env = %#v", request.RuntimeEnv)
	}
	if !strings.Contains(request.Prompt, "$"+"example-app") {
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

	provider := &applicationPromptProvider{}
	registry := agent.NewRegistry()
	if err := registry.Register(provider); err != nil {
		t.Fatal(err)
	}
	issuer := &recordingApplicationIssuer{access: ApplicationToolAccess{
		Env: map[string]string{"REMOTE_APPLICATION_API": "https://remote.test/agent-api/schedules", "REMOTE_APPLICATION_GRANT": "complete-only-token"}, Skills: []servicechat.SkillRef{{Name: "Example App", Command: "example-app", Source: "remote"}},
	}}
	service := New(
		store,
		nil,
		nil,
		runhub.New(store),
		registry,
		WithApplicationToolIssuer(issuer),
		WithAgentPolicy(codexTestAgentPolicy()),
	)

	const scheduledTaskID = "task-123"
	const scheduledRunID = "run-456"
	handle, err := service.Start(StartInput{
		ChatID:                meta.ID,
		Prompt:                `[Scheduled task "watch deploy", fire 3/12] Continue the standing task.`,
		Actor:                 Actor{Email: "owner@example.com"},
		ApplicationInstanceID: scheduledTaskID,
		ApplicationRequestID:  scheduledRunID,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result := awaitScheduleRun(t, handle); result.Err != nil {
		t.Fatalf("run result error = %v", result.Err)
	}

	issued := issuer.request(t, 0)
	if issued.ApplicationInstanceID != scheduledTaskID {
		t.Fatalf("scheduled task ID = %q, want %q", issued.ApplicationInstanceID, scheduledTaskID)
	}
	if issued.ApplicationRequestID != scheduledRunID {
		t.Fatalf("scheduled run ID = %q, want %q", issued.ApplicationRequestID, scheduledRunID)
	}
	request := provider.request(t, 0)
	if len(request.RuntimeEnv) == 0 {
		t.Fatal("scheduled run did not enable completion tooling")
	}
	if request.RuntimeEnv["REMOTE_APPLICATION_GRANT"] != "complete-only-token" {
		t.Fatalf("runtime grant = %q", request.RuntimeEnv["REMOTE_APPLICATION_GRANT"])
	}
	if !strings.Contains(request.Prompt, "$"+"example-app") {
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
	provider := &applicationPromptProvider{
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

	provider := &applicationPromptProvider{
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

type installedScheduleIssuer struct {
	*recordingApplicationIssuer
	available bool
}

func (i installedScheduleIssuer) IssueApplicationTools(ctx context.Context, request ApplicationToolRequest) (ApplicationToolAccess, error) {
	if !i.available {
		return ApplicationToolAccess{}, nil
	}
	return i.recordingApplicationIssuer.IssueApplicationTools(ctx, request)
}
func TestInstalledSchedulerInjectsAccessWithoutSelectedSkill(t *testing.T) {
	for _, installed := range []bool{true, false} {
		t.Run(fmt.Sprintf("installed=%t", installed), func(t *testing.T) {
			ctx := context.Background()
			store, err := filechat.New(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			meta, err := store.Create(ctx, servicechat.Meta{ID: "aabbcc66", Provider: servicechat.ProviderCodex, ProjectID: "project"})
			if err != nil {
				t.Fatal(err)
			}
			provider := &applicationPromptProvider{}
			registry := agent.NewRegistry()
			if err := registry.Register(provider); err != nil {
				t.Fatal(err)
			}
			issuer := installedScheduleIssuer{recordingApplicationIssuer: &recordingApplicationIssuer{access: ApplicationToolAccess{Env: map[string]string{"REMOTE_APPLICATION_API": "https://remote.test/agent-api/applications", "REMOTE_APPLICATION_GRANT": "grant"}, Skills: []servicechat.SkillRef{{Name: "Example App", Command: "example-app", Source: "remote"}}}}, available: installed}
			service := New(store, nil, nil, runhub.New(store), registry, WithApplicationToolIssuer(issuer), WithAgentPolicy(codexTestAgentPolicy()))
			handle, err := service.Start(StartInput{ChatID: meta.ID, Prompt: "notify me daily at 15:00 UTC", Actor: Actor{Email: "owner@example.com"}}, nil)
			if err != nil {
				t.Fatal(err)
			}
			if result := awaitScheduleRun(t, handle); result.Err != nil {
				t.Fatal(result.Err)
			}
			request := provider.request(t, 0)
			if (len(request.RuntimeEnv) > 0) != installed {
				t.Fatalf("tools enabled=%t", len(request.RuntimeEnv) > 0)
			}
			if installed && (request.RuntimeEnv["REMOTE_APPLICATION_API"] == "" || request.RuntimeEnv["REMOTE_APPLICATION_GRANT"] == "" || !strings.Contains(request.Prompt, "$example-app")) {
				t.Fatal("installed app missing runtime access or skill")
			}
			if !installed && len(request.RuntimeEnv) != 0 {
				t.Fatal("uninstalled app received runtime grant")
			}
		})
	}
}
