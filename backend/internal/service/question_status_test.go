package service

import (
	"context"
	"errors"
	"testing"
	"time"

	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
	"github.com/futrx-com/remote.futrx.com/internal/service/workspacehub"
	"github.com/futrx-com/remote.futrx.com/internal/stores/filechat"
)

type questionStatusRuns struct{ running bool }

func (r *questionStatusRuns) IsRunning(servicechat.ID) bool { return r.running }
func (r *questionStatusRuns) Cancel(context.Context, servicechat.ID) error {
	r.running = false
	return nil
}

func newQuestionStatusChat(t *testing.T) (notifyingChatRepository, *questionStatusRuns, *servicechat.Service) {
	t.Helper()
	store, err := filechat.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	runs := &questionStatusRuns{running: true}
	repo := notifyingChatRepository{Repository: store, workspace: workspacehub.New(), running: runs.IsRunning}
	if _, err := repo.Create(context.Background(), servicechat.Meta{ID: "abcd", Title: "Chat"}); err != nil {
		t.Fatal(err)
	}
	return repo, runs, servicechat.New(repo, nil, nil, runs, servicechat.WithCopiedEventAppender(repo))
}

func TestQuestionStatusPublishesWithoutPushAndDoesNotUnlockTheRun(t *testing.T) {
	ctx := context.Background()
	repo, runs, chats := newQuestionStatusChat(t)
	sub := repo.workspace.Subscribe()
	defer sub.Close()
	appendEvent := func(event servicechat.Event) {
		t.Helper()
		event.TurnID = "turn"
		if _, err := repo.AppendEvent(ctx, "abcd", event); err != nil {
			t.Fatal(err)
		}
	}
	appendEvent(servicechat.Event{Type: "interaction_request", Status: "user_input", ID: "question-1"})
	select {
	case event := <-sub.Events():
		if event.Type != "chat.upsert" || event.Chat == nil || !event.Chat.AwaitingAnswer || !event.Chat.Running {
			t.Fatalf("question workspace update = %+v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("question did not publish a workspace update")
	}
	appendEvent(servicechat.Event{Type: "interaction_request", Status: "user_input", ID: "question-2"})
	meta, err := chats.MarkRead(ctx, "abcd")
	if err != nil || !meta.AwaitingAnswer {
		t.Fatalf("reading cleared the question: %+v, %v", meta, err)
	}
	appendEvent(servicechat.Event{Type: "interaction_resolved", Status: "approved", ID: "approval"})
	appendEvent(servicechat.Event{Type: "interaction_resolved", Status: "answered", ID: "question-1"})
	meta, err = chats.Get(ctx, "abcd")
	if err != nil || !meta.AwaitingAnswer || !meta.Running {
		t.Fatalf("partial answer cleared remaining question or run lock: %+v, %v", meta, err)
	}
	appendEvent(servicechat.Event{Type: "interaction_resolved", Status: "answered", ID: "question-2"})
	meta, err = chats.Get(ctx, "abcd")
	if err != nil || meta.AwaitingAnswer || !meta.Running {
		t.Fatalf("answer did not clear status while preserving run: %+v, %v", meta, err)
	}
	appendEvent(servicechat.Event{Type: "interaction_request", Status: "user_input", ID: "question-3"})
	runs.running = false
	meta, err = chats.Get(ctx, "abcd")
	if err != nil || meta.AwaitingAnswer {
		t.Fatalf("dead native request remained actionable: %+v, %v", meta, err)
	}
}

func TestLegacyQuestionStatusSurvivesCompletionButNotForkOrRewind(t *testing.T) {
	ctx := context.Background()
	repo, runs, chats := newQuestionStatusChat(t)
	for _, event := range []servicechat.Event{
		{T: 1, Type: "tool_use_start", Name: "AskUserQuestion", ID: "question", TurnID: "turn"},
		{T: 2, Type: "complete", TurnID: "turn"},
	} {
		if _, err := repo.AppendEvent(ctx, "abcd", event); err != nil {
			t.Fatal(err)
		}
	}
	runs.running = false
	meta, err := chats.Get(ctx, "abcd")
	if err != nil || !meta.AwaitingAnswer {
		t.Fatalf("completed legacy question lost status: %+v, %v", meta, err)
	}
	fork, err := chats.Fork(ctx, "abcd")
	if err != nil || fork.AwaitingAnswer || fork.QuestionState.AwaitingAnswer(true) {
		t.Fatalf("copied question activated fork status: %+v, %v", fork, err)
	}
	if _, err := chats.Rewind(ctx, "abcd", 2); err != nil {
		t.Fatal(err)
	}
	meta, err = chats.Get(ctx, "abcd")
	if err != nil || meta.AwaitingAnswer {
		t.Fatalf("retained historical question activated rewound status: %+v, %v", meta, err)
	}
}

type failingQuestionProjection struct {
	*chatRepoStub
	err error
}

func (r failingQuestionProjection) Update(context.Context, servicechat.ID, func(*servicechat.Meta)) (servicechat.Meta, error) {
	return servicechat.Meta{}, r.err
}

func TestQuestionMetadataFailureIsReported(t *testing.T) {
	failure := errors.New("metadata write failed")
	repo := notifyingChatRepository{
		Repository: failingQuestionProjection{chatRepoStub: &chatRepoStub{}, err: failure},
		workspace:  workspacehub.New(),
	}
	if _, err := repo.AppendEvent(context.Background(), "abcd", servicechat.Event{
		Type: "interaction_request", Status: "user_input", ID: "question",
	}); !errors.Is(err, failure) {
		t.Fatalf("question metadata failure = %v, want %v", err, failure)
	}
}
