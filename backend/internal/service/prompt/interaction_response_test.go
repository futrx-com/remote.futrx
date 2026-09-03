package prompt

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
	"github.com/futrx-com/remote.futrx.com/internal/service/runhub"
	"github.com/futrx-com/remote.futrx.com/internal/stores/filechat"
)

type interactionResponseProvider struct {
	started  chan struct{}
	received chan agent.InteractionResponse
}

func (p *interactionResponseProvider) ID() agent.ProviderID { return agent.ProviderCodex }

func (p *interactionResponseProvider) Capabilities(
	context.Context,
	agent.CapabilityRequest,
) (agent.Capabilities, error) {
	return agent.Capabilities{Provider: agent.ProviderCodex}, nil
}

func (p *interactionResponseProvider) Run(
	ctx context.Context,
	req agent.RunRequest,
	emit func(agent.Event),
) error {
	close(p.started)
	select {
	case response := <-req.InteractionResponses:
		p.received <- response
	case <-ctx.Done():
		return ctx.Err()
	}
	emit(agent.Event{Type: agent.EventRunCompleted})
	return nil
}

func TestRespondInteractionRoutesAnswerToActiveRun(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	store, err := filechat.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	meta, err := store.Create(ctx, servicechat.Meta{
		ID:       "aabbccddeeff",
		Provider: servicechat.ProviderCodex,
		Cwd:      t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}

	provider := &interactionResponseProvider{
		started:  make(chan struct{}),
		received: make(chan agent.InteractionResponse, 1),
	}
	registry := agent.NewRegistry()
	if err := registry.Register(provider); err != nil {
		t.Fatal(err)
	}
	service := New(store, nil, nil, runhub.New(store), registry)
	handle, err := service.Start(StartInput{ChatID: meta.ID, Prompt: "continue"}, nil)
	if err != nil {
		t.Fatal(err)
	}

	select {
	case <-provider.started:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}

	want := agent.InteractionResponse{
		ID:     `"request-42"`,
		Result: json.RawMessage(`{"decision":"accept"}`),
	}
	if err := service.RespondInteraction(meta.ID, want); err != nil {
		t.Fatal(err)
	}

	select {
	case got := <-provider.received:
		if got.ID != want.ID || string(got.Result) != string(want.Result) || len(got.Error) != 0 {
			t.Fatalf("response = %#v, want %#v", got, want)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}

	select {
	case result := <-handle.Done:
		if result.Err != nil {
			t.Fatal(result.Err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}
