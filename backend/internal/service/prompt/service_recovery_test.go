package prompt

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	agentmodule "github.com/futrx-com/remote.futrx.com/internal/service/agent/module"
	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
	"github.com/futrx-com/remote.futrx.com/internal/service/runhub"
	"github.com/futrx-com/remote.futrx.com/internal/stores/filechat"
)

type recoveryProvider struct {
	requests  []agent.RunRequest
	failFirst bool
}

func (p *recoveryProvider) ID() agent.ProviderID                     { return agent.ProviderCodex }
func (p *recoveryProvider) Parser(agent.RunRequest) agent.LineParser { return nil }
func (p *recoveryProvider) Capabilities(context.Context, agent.CapabilityRequest) (agent.Capabilities, error) {
	return agent.Capabilities{Provider: agent.ProviderCodex}, nil
}
func (p *recoveryProvider) Run(_ context.Context, req agent.RunRequest, emit func(agent.Event)) error {
	p.requests = append(p.requests, req)
	if p.failFirst && len(p.requests) == 1 {
		return agent.ErrSessionNotFound
	}
	emit(agent.Event{
		T: time.Now().UnixMilli(), Type: agent.EventSessionUpdated,
		Provider: agent.ProviderCodex, SessionID: "new-thread",
	})
	emit(agent.Event{T: time.Now().UnixMilli(), Type: agent.EventAssistantTextDelta, Text: "recovered"})
	emit(agent.Event{T: time.Now().UnixMilli(), Type: agent.EventRunCompleted})
	return nil
}

func TestRunPromptRecoversMissingCodexSessionFromVisibleTranscript(t *testing.T) {
	ctx := context.Background()
	store, err := filechat.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	meta, err := store.Create(ctx, servicechat.Meta{
		ID: "abcdef123456", Title: "existing", Provider: servicechat.ProviderCodex,
		CodexSessionID: "missing-thread", Cwd: t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range []servicechat.Event{
		{T: 1, Type: "user", Text: "earlier question"},
		{T: 2, Type: "assistant_text", Text: "earlier answer"},
		{T: 3, Type: "complete"},
	} {
		if _, err := store.AppendEvent(ctx, meta.ID, event); err != nil {
			t.Fatal(err)
		}
	}

	provider := &recoveryProvider{failFirst: true}
	registry := agent.NewRegistry()
	if err := registry.Register(provider); err != nil {
		t.Fatal(err)
	}
	service := New(store, nil, nil, runhub.New(store), registry)
	emit := func(event ChatEvent) {
		if _, err := store.AppendEvent(ctx, meta.ID, event); err != nil {
			t.Fatal(err)
		}
	}
	service.runPrompt(ctx, meta.ID, "current question", emit, emit)

	if len(provider.requests) != 2 {
		t.Fatalf("requests = %d, want stale resume plus one retry", len(provider.requests))
	}
	if provider.requests[0].ResumeID != "missing-thread" || provider.requests[1].ResumeID != "" {
		t.Fatalf("resume ids = %q, %q", provider.requests[0].ResumeID, provider.requests[1].ResumeID)
	}
	for _, want := range []string{"earlier question", "earlier answer", "Current user request:\ncurrent question"} {
		if !strings.Contains(provider.requests[1].Prompt, want) {
			t.Fatalf("recovery prompt missing %q:\n%s", want, provider.requests[1].Prompt)
		}
	}

	gotMeta, err := store.Get(ctx, meta.ID)
	if err != nil {
		t.Fatal(err)
	}
	if gotMeta.CodexSessionID != "new-thread" {
		t.Fatalf("session id = %q, want new-thread", gotMeta.CodexSessionID)
	}
	events, err := store.ReadEvents(ctx, meta.ID)
	if err != nil {
		t.Fatal(err)
	}
	foundRecovery, foundAnswer := false, false
	for _, event := range events {
		foundRecovery = foundRecovery || event.Type == "system" && event.Subtype == "session_recovered"
		foundAnswer = foundAnswer || event.Type == "assistant_text" && event.Text == "recovered"
		if event.Type == "error" {
			t.Fatalf("unexpected recovery error: %s", event.Message)
		}
	}
	if !foundRecovery || !foundAnswer {
		t.Fatalf("events missing recovery markers: %#v", events)
	}
}

func TestRunPromptDoesNotResumeWhenModuleDisablesSessions(t *testing.T) {
	ctx := context.Background()
	store, err := filechat.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	meta, err := store.Create(ctx, servicechat.Meta{
		ID: "abcdef123456", Provider: servicechat.ProviderCodex,
		CodexSessionID: "old-thread", Cwd: t.TempDir(), ForkPending: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendEvent(ctx, meta.ID, servicechat.Event{
		T: 1, Type: "assistant_text", Text: "visible history",
	}); err != nil {
		t.Fatal(err)
	}

	provider := &recoveryProvider{}
	registry := agent.NewRegistry()
	if err := registry.Register(provider); err != nil {
		t.Fatal(err)
	}
	policy := testAgentPolicy{"codex": {
		ID: agent.ProviderCodex, Label: "Codex",
		ExecutionScopes: []agentmodule.ExecutionScope{agentmodule.ScopeHost},
		Features:        agentmodule.Features{Skills: agentmodule.SkillsNone},
	}}
	service := New(store, nil, nil, runhub.New(store), registry, WithAgentPolicy(policy))
	service.runPrompt(ctx, meta.ID, "continue", func(ChatEvent) {}, func(ChatEvent) {})

	if len(provider.requests) != 1 {
		t.Fatalf("requests = %d, want 1", len(provider.requests))
	}
	request := provider.requests[0]
	if request.ResumeID != "" {
		t.Fatalf("resume ID = %q, want empty", request.ResumeID)
	}
	if request.Fork {
		t.Fatal("fork = true for module that disables native sessions")
	}
	if !strings.Contains(request.Prompt, "visible history") {
		t.Fatalf("fresh prompt does not include visible history: %q", request.Prompt)
	}
}

func TestProjectRunUsesContainerWorkspaceInsteadOfHostPath(t *testing.T) {
	ctx := context.Background()
	store, err := filechat.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	meta, err := store.Create(ctx, servicechat.Meta{
		ID:        "abcdef123456",
		Provider:  servicechat.ProviderCodex,
		ProjectID: "project-1",
		Cwd:       "/var/lib/remote/projects/example/workspace",
	})
	if err != nil {
		t.Fatal(err)
	}

	provider := &recoveryProvider{}
	registry := agent.NewRegistry()
	if err := registry.Register(provider); err != nil {
		t.Fatal(err)
	}
	policy := testAgentPolicy{"codex": {
		ID:              agent.ProviderCodex,
		Label:           "Codex",
		ExecutionScopes: []agentmodule.ExecutionScope{agentmodule.ScopeProject},
	}}
	service := New(store, nil, nil, runhub.New(store), registry, WithAgentPolicy(policy))
	if err := service.runPrompt(ctx, meta.ID, "use the browser", func(ChatEvent) {}, func(ChatEvent) {}); err != nil {
		t.Fatal(err)
	}

	if len(provider.requests) != 1 {
		t.Fatalf("requests = %d, want 1", len(provider.requests))
	}
	if got := provider.requests[0].Cwd; got != agent.ProjectWorkspacePath {
		t.Fatalf("project run cwd = %q, want %q", got, agent.ProjectWorkspacePath)
	}
}

func TestClearSessionIDForProvider(t *testing.T) {
	meta := &ChatMeta{
		Sessions:        servicechat.SessionIDs{"future-agent": "future", servicechat.ProviderCodex: "o"},
		ClaudeSessionID: "c",
		CodexSessionID:  "o",
		KimiSessionID:   "k",
	}
	clearSessionIDForProvider(meta, agent.ProviderCodex)
	if meta.CodexSessionID != "" || meta.ClaudeSessionID != "c" || meta.KimiSessionID != "k" || meta.Sessions["future-agent"] != "future" {
		t.Fatalf("wrong provider session cleared: %#v", meta)
	}
}

func TestRunPromptRejectsProviderOutsideChatScope(t *testing.T) {
	ctx := context.Background()
	store, err := filechat.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	meta, err := store.Create(ctx, servicechat.Meta{
		ID: "abcdef123456", Provider: servicechat.ProviderCodex, ProjectID: "abcd", Cwd: t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	provider := &recoveryProvider{}
	registry := agent.NewRegistry()
	if err := registry.Register(provider); err != nil {
		t.Fatal(err)
	}
	policy := testAgentPolicy{"codex": {
		ID: agent.ProviderCodex, Label: "Codex", ExecutionScopes: []agentmodule.ExecutionScope{agentmodule.ScopeHost},
	}}
	service := New(store, nil, nil, runhub.New(store), registry, WithAgentPolicy(policy))
	err = service.runPrompt(ctx, meta.ID, "do work", func(ChatEvent) {}, func(ChatEvent) {})
	if err != ErrUnsupportedAgentScope {
		t.Fatalf("runPrompt error = %v, want ErrUnsupportedAgentScope", err)
	}
	if len(provider.requests) != 0 {
		t.Fatalf("provider ran outside its scope: %#v", provider.requests)
	}
}
