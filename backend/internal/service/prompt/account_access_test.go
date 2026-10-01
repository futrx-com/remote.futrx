package prompt

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	"github.com/futrx-com/remote.futrx.com/internal/rbac"
)

type promptAccountCheck struct {
	mu     sync.Mutex
	actors []string
	ids    []string
	denyAt int
}

func (a *promptAccountCheck) Resolve(ctx context.Context, _ agent.ProviderID, id string) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	actor, ok := rbac.ActorFromContext(ctx)
	if !ok {
		return "", rbac.ErrActorRequired
	}
	a.actors = append(a.actors, actor.Email)
	a.ids = append(a.ids, id)
	if actor.Email == "" || (a.denyAt > 0 && len(a.ids) >= a.denyAt) {
		return "", rbac.ErrDenied
	}
	return "team", nil
}

type accountRecordingProvider struct {
	usageProvider
	requests []agent.RunRequest
	actors   []string
}

func (p *accountRecordingProvider) Run(ctx context.Context, req agent.RunRequest, emit func(agent.Event)) error {
	p.requests = append(p.requests, req)
	actor, _ := rbac.ActorFromContext(ctx)
	p.actors = append(p.actors, actor.Email)
	return p.usageProvider.Run(ctx, req, emit)
}
func TestPromptAccountChecksKeepScheduledActorAndPinResolvedAccount(t *testing.T) {
	for _, scheduled := range []bool{false, true} {
		provider := &accountRecordingProvider{}
		access := &promptAccountCheck{}
		service, _, meta := newUsagePromptService(t, provider, &recordingLedger{}, WithAccountAccess(access), WithScheduleToolIssuer(stubScheduleTools{}))
		input := StartInput{ChatID: meta.ID, Prompt: "hello", Actor: Actor{Email: "owner@example.com", IsAdmin: true}}
		if scheduled {
			input.ScheduledTaskID = "scheduled-one"
		}
		handle, err := service.Start(input, nil)
		if err != nil {
			t.Fatal(err)
		}
		if result := <-handle.Done; result.Err != nil {
			t.Fatal(result.Err)
		}
		if len(provider.requests) != 1 || provider.requests[0].AccountID != "team" || provider.actors[0] != "owner@example.com" {
			t.Fatalf("provider request: %+v actors=%v", provider.requests, provider.actors)
		}
		access.mu.Lock()
		if len(access.ids) < 3 || access.ids[len(access.ids)-1] != "team" {
			t.Fatalf("missing run recheck: %v", access.ids)
		}
		for _, email := range access.actors {
			if email != "owner@example.com" {
				t.Fatalf("wrong actor: %v", access.actors)
			}
		}
		access.mu.Unlock()
	}
}
func TestPromptDenialBeforeAcceptanceAndRevocationBeforeRunLeaveNoPrompt(t *testing.T) {
	for _, denyAt := range []int{1, 2} {
		provider := &accountRecordingProvider{}
		access := &promptAccountCheck{denyAt: denyAt}
		service, store, meta := newUsagePromptService(t, provider, &recordingLedger{}, WithAccountAccess(access))
		handle, err := service.Start(StartInput{ChatID: meta.ID, Prompt: "must not run", Actor: Actor{Email: "member@example.com"}}, nil)
		if denyAt == 1 {
			if !errors.Is(err, rbac.ErrDenied) {
				t.Fatalf("preflight: %v", err)
			}
		} else {
			if err != nil {
				t.Fatal(err)
			}
			if result := <-handle.Done; !errors.Is(result.Err, rbac.ErrDenied) {
				t.Fatalf("revocation: %v", result.Err)
			}
		}
		events, err := store.ReadEvents(context.Background(), meta.ID)
		if err != nil || len(events) != 0 || len(provider.requests) != 0 {
			t.Fatalf("denied run produced effects: events=%v calls=%d err=%v", events, len(provider.requests), err)
		}
	}
}
