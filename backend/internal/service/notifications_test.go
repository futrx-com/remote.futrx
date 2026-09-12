package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
	servicenotify "github.com/futrx-com/remote.futrx.com/internal/service/notify"
	"github.com/futrx-com/remote.futrx.com/internal/service/prompt"
	serviceschedule "github.com/futrx-com/remote.futrx.com/internal/service/schedule"
)

// capturingSink records the events the notifier hands to a delivery target.
type capturingSink struct {
	mu        sync.Mutex
	delivered []servicenotify.Event
	arrived   chan struct{}
}

func newCapturingSink() *capturingSink {
	return &capturingSink{arrived: make(chan struct{}, 8)}
}

func (s *capturingSink) Name() string { return "capture" }

func (s *capturingSink) Configured(servicenotify.Config) bool { return true }

func (s *capturingSink) Send(_ context.Context, _ servicenotify.Config, event servicenotify.Event) error {
	s.mu.Lock()
	s.delivered = append(s.delivered, event)
	s.mu.Unlock()
	s.arrived <- struct{}{}
	return nil
}

func (s *capturingSink) events() []servicenotify.Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]servicenotify.Event(nil), s.delivered...)
}

func (s *capturingSink) waitFor(t *testing.T, count int) []servicenotify.Event {
	t.Helper()
	for i := 0; i < count; i++ {
		select {
		case <-s.arrived:
		case <-time.After(5 * time.Second):
			t.Fatalf("timed out waiting for delivery %d of %d", i+1, count)
		}
	}
	return s.events()
}

// chatLookupStub satisfies the parts of servicechat.Repository the observer
// uses; the rest panics so an accidental dependency is loud.
type chatLookupStub struct {
	servicechat.Repository
	meta servicechat.Meta
	err  error
}

func (s chatLookupStub) Get(context.Context, servicechat.ID) (servicechat.Meta, error) {
	return s.meta, s.err
}

type enabledStore struct{}

func (enabledStore) Load(context.Context) (servicenotify.Config, error) {
	return servicenotify.Config{
		Enabled:  true,
		Telegram: servicenotify.TelegramConfig{BotToken: "123:abc", ChatID: "-100"},
		Events: servicenotify.EventToggles{
			RunFinished:    true,
			RunFailed:      true,
			NeedsAttention: true,
			ScheduledRun:   true,
		},
	}, nil
}

func (enabledStore) Save(context.Context, servicenotify.Config) error { return nil }

func newObserver(t *testing.T, chat servicechat.Meta) (*notifyObserver, *capturingSink) {
	t.Helper()
	sink := newCapturingSink()
	notifications := servicenotify.New(
		context.Background(),
		enabledStore{},
		"https://remote.example.com",
		servicenotify.WithNotifier(servicenotify.NewNotifier(
			nil,
			servicenotify.WithSinks(sink),
			servicenotify.WithBackoff(time.Millisecond),
		)),
	)
	t.Cleanup(notifications.Stop)
	return &notifyObserver{notifications: notifications, chats: chatLookupStub{meta: chat}}, sink
}

func TestRunSettledMapsOutcomesToEvents(t *testing.T) {
	tests := []struct {
		name        string
		outcome     prompt.RunOutcome
		wantKind    servicenotify.Kind
		wantStatus  string
		wantSummary string
	}{
		{
			name:        "success carries the agent output",
			outcome:     prompt.RunOutcome{ChatID: "abc123", RunID: 1, Output: "  All tests pass.\n\n"},
			wantKind:    servicenotify.KindRunFinished,
			wantStatus:  servicenotify.StatusFinished,
			wantSummary: "All tests pass.",
		},
		{
			name:        "failure carries the error",
			outcome:     prompt.RunOutcome{ChatID: "abc123", RunID: 2, Err: errors.New("claude exit: status 1")},
			wantKind:    servicenotify.KindRunFailed,
			wantStatus:  servicenotify.StatusFailed,
			wantSummary: "claude exit: status 1",
		},
		{
			name:        "cancellation is reported as a failure",
			outcome:     prompt.RunOutcome{ChatID: "abc123", RunID: 3, Cancelled: true, Output: "partial"},
			wantKind:    servicenotify.KindRunFailed,
			wantStatus:  servicenotify.StatusCancelled,
			wantSummary: "The run was cancelled before it finished.",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			observer, sink := newObserver(t, servicechat.Meta{
				ID:       "abc123",
				Title:    "Fix login",
				Provider: servicechat.ProviderClaude,
			})

			observer.RunSettled(context.Background(), test.outcome)
			events := sink.waitFor(t, 1)

			got := events[0]
			if got.Event != test.wantKind || got.Status != test.wantStatus {
				t.Fatalf("event = (%q, %q), want (%q, %q)", got.Event, got.Status, test.wantKind, test.wantStatus)
			}
			if got.Summary != test.wantSummary {
				t.Fatalf("summary = %q, want %q", got.Summary, test.wantSummary)
			}
			if got.ChatID != "abc123" || got.ChatTitle != "Fix login" || got.Provider != "claude" {
				t.Fatalf("chat identity = %+v", got)
			}
			if got.URL != "https://remote.example.com/?chat=abc123" {
				t.Fatalf("deep link = %q", got.URL)
			}
		})
	}
}

func TestRunSettledLeavesScheduledRunsToTheScheduler(t *testing.T) {
	observer, sink := newObserver(t, servicechat.Meta{ID: "abc123"})

	observer.RunSettled(context.Background(), prompt.RunOutcome{
		ChatID:          "abc123",
		RunID:           1,
		ScheduledTaskID: "task-1",
	})

	select {
	case <-sink.arrived:
		t.Fatal("a scheduled run produced an interactive run notification")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestRunSettledDeliversOncePerRun(t *testing.T) {
	observer, sink := newObserver(t, servicechat.Meta{ID: "abc123"})
	outcome := prompt.RunOutcome{ChatID: "abc123", RunID: 9, Output: "done"}

	observer.RunSettled(context.Background(), outcome)
	observer.RunSettled(context.Background(), outcome)

	events := sink.waitFor(t, 1)
	time.Sleep(100 * time.Millisecond)
	if got := sink.events(); len(got) != 1 {
		t.Fatalf("delivered %d events, want 1: %+v", len(got), events)
	}
}

func TestRunToolStartedOnlyReportsHumanFacingTools(t *testing.T) {
	tests := []struct {
		name     string
		toolName string
		wantPing bool
	}{
		{name: "plan approval pings", toolName: "ExitPlanMode", wantPing: true},
		{name: "question pings", toolName: "AskUserQuestion", wantPing: true},
		{name: "file edit stays quiet", toolName: "Edit", wantPing: false},
		{name: "shell stays quiet", toolName: "Bash", wantPing: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			observer, sink := newObserver(t, servicechat.Meta{ID: "abc123", Title: "Ship it"})

			observer.RunToolStarted(context.Background(), "abc123", test.toolName)

			if !test.wantPing {
				select {
				case <-sink.arrived:
					t.Fatalf("%q should not notify", test.toolName)
				case <-time.After(100 * time.Millisecond):
				}
				return
			}
			events := sink.waitFor(t, 1)
			if events[0].Event != servicenotify.KindNeedsAttention ||
				events[0].Status != servicenotify.StatusWaiting {
				t.Fatalf("event = %+v", events[0])
			}
		})
	}
}

func TestScheduledRunFinishedReportsTaskOutcomes(t *testing.T) {
	tests := []struct {
		name        string
		result      serviceschedule.RunResult
		wantStatus  string
		wantSummary string
	}{
		{
			name:        "success",
			result:      serviceschedule.RunResult{Output: "nightly build green"},
			wantStatus:  servicenotify.StatusSucceeded,
			wantSummary: "Task: Nightly build\nnightly build green",
		},
		{
			name:        "failure",
			result:      serviceschedule.RunResult{Err: errors.New("container did not start")},
			wantStatus:  servicenotify.StatusFailed,
			wantSummary: "Task: Nightly build\ncontainer did not start",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			observer, sink := newObserver(t, servicechat.Meta{ID: "abc123", Title: "Nightly"})

			observer.ScheduledRunFinished(context.Background(), serviceschedule.Task{
				ID:        "000000000000000000000001",
				Name:      "Nightly build",
				ChatID:    "abc123",
				LastRunAt: 1700000000000,
			}, test.result)

			events := sink.waitFor(t, 1)
			got := events[0]
			if got.Event != servicenotify.KindScheduledRun || got.Status != test.wantStatus {
				t.Fatalf("event = (%q, %q)", got.Event, got.Status)
			}
			if got.Summary != test.wantSummary {
				t.Fatalf("summary = %q, want %q", got.Summary, test.wantSummary)
			}
		})
	}
}

func TestObserverSurvivesChatLookupFailures(t *testing.T) {
	sink := newCapturingSink()
	notifications := servicenotify.New(
		context.Background(),
		enabledStore{},
		"https://remote.example.com",
		servicenotify.WithNotifier(servicenotify.NewNotifier(
			nil,
			servicenotify.WithSinks(sink),
			servicenotify.WithBackoff(time.Millisecond),
		)),
	)
	t.Cleanup(notifications.Stop)
	observer := &notifyObserver{
		notifications: notifications,
		chats:         chatLookupStub{err: errors.New("chat store unavailable")},
	}

	observer.RunSettled(context.Background(), prompt.RunOutcome{ChatID: "abc123", RunID: 1, Output: "done"})

	events := sink.waitFor(t, 1)
	if events[0].ChatID != "abc123" || events[0].ChatTitle != "" {
		t.Fatalf("event = %+v, want the id without enrichment", events[0])
	}
}
