package prompt

import (
	"context"
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
	"github.com/futrx-com/remote.futrx.com/internal/service/runhub"
	"github.com/futrx-com/remote.futrx.com/internal/stores/filechat"
)

type notificationInstructionProvider struct {
	id         agent.ProviderID
	requests   []agent.RunRequest
	failResume bool
}

func (p *notificationInstructionProvider) ID() agent.ProviderID { return p.id }
func (p *notificationInstructionProvider) Capabilities(context.Context, agent.CapabilityRequest) (agent.Capabilities, error) {
	return agent.Capabilities{Provider: p.id}, nil
}
func (p *notificationInstructionProvider) Run(_ context.Context, req agent.RunRequest, emit func(agent.Event)) error {
	p.requests = append(p.requests, req)
	if p.failResume && req.ResumeID != "" {
		return agent.ErrSessionNotFound
	}
	emit(agent.Event{Type: agent.EventAssistantTextDelta, Text: "Which region should I use? <notifi"})
	emit(agent.Event{Type: agent.EventAssistantTextDelta, Text: "cation_summary>Choose a deployment region.</notification_summary>"})
	emit(agent.Event{Type: agent.EventRunCompleted})
	return nil
}

func TestNotificationSummaryDoesNotModifyProviderPrompts(t *testing.T) {
	for _, providerID := range []agent.ProviderID{"claude", "codex", "minimax", "kimi", "antigravity"} {
		for _, projectID := range []servicechat.ProjectID{"", "project"} {
			for _, session := range []string{"fresh", "resumed", "recovered"} {
				t.Run(string(providerID)+"/"+string(projectID)+"/"+session, func(t *testing.T) {
					ctx := context.Background()
					store, err := filechat.New(t.TempDir())
					if err != nil {
						t.Fatal(err)
					}
					t.Cleanup(func() { _ = store.Close() })
					meta := servicechat.Meta{ID: "abcd", Provider: providerID, ProjectID: projectID}
					if session != "fresh" {
						meta.SetSessionID(providerID, "existing-session")
					}
					if _, err := store.Create(ctx, meta); err != nil {
						t.Fatal(err)
					}
					provider := &notificationInstructionProvider{id: providerID, failResume: session == "recovered"}
					registry := agent.NewRegistry()
					if err := registry.Register(provider); err != nil {
						t.Fatal(err)
					}
					service := New(store, nil, nil, runhub.New(store), registry)
					var summary string
					emit := func(event ChatEvent) {
						if event.Type == "complete" {
							summary = event.NotificationSummary
						}
						if _, err := store.AppendEvent(ctx, meta.ID, event); err != nil {
							t.Fatal(err)
						}
					}
					if err := service.runPrompt(ctx, meta.ID, "help deploy", emit, emit); err != nil {
						t.Fatal(err)
					}
					wantRequests := 1
					if session == "recovered" {
						wantRequests = 2
					}
					if len(provider.requests) != wantRequests {
						t.Fatalf("provider requests = %d, want %d", len(provider.requests), wantRequests)
					}
					for _, request := range provider.requests {
						if request.Prompt != "help deploy" {
							t.Fatalf("provider prompt = %q, want the original user prompt", request.Prompt)
						}
					}
					if summary != "Choose a deployment region." {
						t.Fatalf("plain question summary = %q", summary)
					}
					stored, err := store.ReadEvents(ctx, meta.ID)
					if err != nil {
						t.Fatal(err)
					}
					var visible string
					for _, event := range stored {
						if event.Type == "user" && event.Text != "help deploy" {
							t.Fatalf("notification instructions leaked into the user message: %s", event.Text)
						}
						if event.Type == "assistant_text" {
							visible += event.Text
						}
					}
					if visible != "Which region should I use? " {
						t.Fatalf("private question summary leaked into visible text: %q", visible)
					}
				})
			}
		}
	}
}
