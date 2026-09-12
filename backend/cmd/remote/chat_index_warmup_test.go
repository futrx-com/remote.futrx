package main

import (
	"context"
	"errors"
	"log"
	"strings"
	"testing"
	"time"
)

type chatIndexWarmerStub struct {
	calls int
	ctx   context.Context
	limit int
	err   error
}

func (s *chatIndexWarmerStub) WarmRecentChatIndexes(ctx context.Context, limit int) error {
	s.calls++
	s.ctx = ctx
	s.limit = limit
	return s.err
}

type chatIndexWarmerFunc func(context.Context, int) error

func (f chatIndexWarmerFunc) WarmRecentChatIndexes(ctx context.Context, limit int) error {
	return f(ctx, limit)
}

func TestStartChatIndexWarmupDoesNotWaitForWarmup(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	released := false
	t.Cleanup(func() {
		if !released {
			close(release)
		}
	})
	warmer := chatIndexWarmerFunc(func(context.Context, int) error {
		close(started)
		<-release
		return nil
	})
	var logs strings.Builder
	returned := make(chan (<-chan struct{}), 1)

	go func() {
		returned <- startChatIndexWarmup(
			context.Background(),
			warmer,
			10,
			log.New(&logs, "", 0),
		)
	}()

	var done <-chan struct{}
	select {
	case done = <-returned:
	case <-time.After(time.Second):
		t.Fatal("startup waited for chat index warmup")
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("chat index warmup did not start")
	}
	select {
	case <-done:
		t.Fatal("chat index warmup finished while the warmer was blocked")
	default:
	}

	close(release)
	released = true
	awaitChatIndexWarmup(t, done)
}

func TestStartChatIndexWarmupRunsOnceWithConfiguredLimit(t *testing.T) {
	type contextKey string
	ctx := context.WithValue(context.Background(), contextKey("source"), "startup")
	warmer := &chatIndexWarmerStub{}
	var logs strings.Builder

	done := startChatIndexWarmup(ctx, warmer, 10, log.New(&logs, "", 0))
	awaitChatIndexWarmup(t, done)

	if warmer.calls != 1 {
		t.Fatalf("warmup calls = %d, want 1", warmer.calls)
	}
	if warmer.ctx != ctx {
		t.Fatal("warmup did not receive the startup context")
	}
	if warmer.limit != 10 {
		t.Fatalf("warmup chat limit = %d, want 10", warmer.limit)
	}
	if logs.Len() != 0 {
		t.Fatalf("successful warmup logged a warning: %q", logs.String())
	}
}

func TestStartChatIndexWarmupLogsFailure(t *testing.T) {
	warmer := &chatIndexWarmerStub{err: errors.New("index unavailable")}
	var logs strings.Builder

	done := startChatIndexWarmup(
		context.Background(),
		warmer,
		10,
		log.New(&logs, "", 0),
	)
	awaitChatIndexWarmup(t, done)

	want := "chat event index warmup warning: index unavailable\n"
	if logs.String() != want {
		t.Fatalf("warmup log = %q, want %q", logs.String(), want)
	}
}

func awaitChatIndexWarmup(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for chat index warmup")
	}
}
