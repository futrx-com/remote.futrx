package chat

import (
	"context"
	"testing"

	permission "github.com/futrx-com/remote.futrx.com/internal/rbac"
)

func TestForkBeforeCopiesOnlyEarlierEventsAndStartsAFreshSession(t *testing.T) {
	repo := &forkRepository{
		source: Meta{
			ID:       "deadbeef",
			Title:    "Source",
			Provider: ProviderCodex,
			Sessions: SessionIDs{ProviderCodex: "codex-session", ProviderClaude: "claude-session"},
		},
		events: []Event{
			{Seq: 1, T: 100, Type: "user"},
			{Seq: 2, T: 110, Type: "complete"},
			{Seq: 3, T: 200, Type: "user"},
			{Seq: 4, T: 210, Type: "complete"},
		},
	}
	service := New(repo, nil, nil, nil,
		WithCopiedEventAppender(repo),
		WithSessionPolicy(forkSessionPolicy{string(ProviderCodex): true}),
	)

	forked, err := service.ForkBefore(permission.ContextWithSystemActor(context.Background()), "deadbeef", 200)
	if err != nil {
		t.Fatal(err)
	}
	if len(repo.copied) != 2 || repo.copied[0].T != 100 || repo.copied[1].T != 110 {
		t.Fatalf("copied events = %#v, want the two before t=200", repo.copied)
	}
	if forked.ForkPending || forked.SessionID(ProviderCodex) != "" || forked.SessionID(ProviderClaude) != "" {
		t.Fatalf("partial fork kept a provider session: %#v", forked)
	}
}

func TestForkBeforeZeroCopiesEverythingLikeFork(t *testing.T) {
	repo := &forkRepository{
		source: Meta{
			ID:       "deadbeef",
			Title:    "Source",
			Provider: ProviderCodex,
			Sessions: SessionIDs{ProviderCodex: "codex-session"},
		},
		events: []Event{{T: 100, Type: "user"}, {T: 200, Type: "user"}},
	}
	service := New(repo, nil, nil, nil,
		WithCopiedEventAppender(repo),
		WithSessionPolicy(forkSessionPolicy{string(ProviderCodex): true}),
	)

	forked, err := service.ForkBefore(permission.ContextWithSystemActor(context.Background()), "deadbeef", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(repo.copied) != 2 {
		t.Fatalf("copied %d events, want 2", len(repo.copied))
	}
	if !forked.ForkPending || forked.SessionID(ProviderCodex) != "codex-session" {
		t.Fatalf("full fork lost the native session: %#v", forked)
	}
}

func TestForkBeforeRejectsNegativeTimestamp(t *testing.T) {
	service := New(&forkRepository{}, nil, nil, nil)
	if _, err := service.ForkBefore(permission.ContextWithSystemActor(context.Background()), "deadbeef", -1); err != ErrInvalidRewindTimestamp {
		t.Fatalf("error = %v, want ErrInvalidRewindTimestamp", err)
	}
}
