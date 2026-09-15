package service

import (
	"context"
	"errors"
	"testing"

	applicationlifecycle "github.com/futrx-com/remote.futrx.com/internal/lifecycle"
	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
	"github.com/futrx-com/remote.futrx.com/internal/service/workspacehub"
)

type lifecycleChatRepositoryStub struct {
	servicechat.Repository
	createResult   servicechat.Meta
	updateResult   servicechat.Meta
	getResult      servicechat.Meta
	truncateResult []servicechat.Event
	err            error
}

func (r lifecycleChatRepositoryStub) Create(context.Context, servicechat.Meta) (servicechat.Meta, error) {
	return r.createResult, r.err
}

func (r lifecycleChatRepositoryStub) Update(
	context.Context,
	servicechat.ID,
	func(*servicechat.Meta),
) (servicechat.Meta, error) {
	return r.updateResult, r.err
}

func (r lifecycleChatRepositoryStub) Delete(context.Context, servicechat.ID) error {
	return r.err
}

func (r lifecycleChatRepositoryStub) Get(context.Context, servicechat.ID) (servicechat.Meta, error) {
	return r.getResult, r.err
}

func (r lifecycleChatRepositoryStub) AppendEvent(
	_ context.Context,
	_ servicechat.ID,
	event servicechat.Event,
) (servicechat.Event, error) {
	event.Seq = 7
	return event, r.err
}

func (r lifecycleChatRepositoryStub) TruncateEventsBefore(
	context.Context,
	servicechat.ID,
	int64,
) ([]servicechat.Event, error) {
	return r.truncateResult, r.err
}

type recordedChatLifecycleEvent struct {
	ctx   context.Context
	event applicationlifecycle.ChatEvent
}

type recordingChatLifecycleSubscriber struct {
	beforeRecord func(applicationlifecycle.ChatEvent)
	events       []recordedChatLifecycleEvent
}

func (s *recordingChatLifecycleSubscriber) OnChat(
	ctx context.Context,
	event applicationlifecycle.ChatEvent,
) {
	if s.beforeRecord != nil {
		s.beforeRecord(event)
	}
	s.events = append(s.events, recordedChatLifecycleEvent{ctx: ctx, event: event})
}

func TestNotifyingChatRepositoryPublishesSuccessfulRecordLifecycle(t *testing.T) {
	publisher := applicationlifecycle.NewChatPublisher()
	workspace := workspacehub.New()
	workspaceEvents := workspace.Subscribe()
	defer workspaceEvents.Close()
	subscriber := &recordingChatLifecycleSubscriber{
		beforeRecord: func(event applicationlifecycle.ChatEvent) {
			assertChatWorkspaceEvent(t, workspaceEvents.Events(), event)
		},
	}
	publisher.Subscribe(subscriber)
	repository := notifyingChatRepository{
		Repository: lifecycleChatRepositoryStub{
			createResult: servicechat.Meta{ID: "deadbeef"},
			updateResult: servicechat.Meta{ID: "cafebabe"},
		},
		workspace: workspace,
		lifecycle: publisher,
	}
	ctx := context.Background()

	if _, err := repository.Create(ctx, servicechat.Meta{}); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.Update(ctx, "cafebabe", func(*servicechat.Meta) {}); err != nil {
		t.Fatal(err)
	}
	if err := repository.Delete(ctx, "0123abcd"); err != nil {
		t.Fatal(err)
	}

	want := []applicationlifecycle.ChatEvent{
		{State: applicationlifecycle.ChatCreated, ChatID: "deadbeef"},
		{State: applicationlifecycle.ChatUpdated, ChatID: "cafebabe"},
		{State: applicationlifecycle.ChatDeleted, ChatID: "0123abcd"},
	}
	assertChatLifecycleEvents(t, subscriber.events, ctx, want)
}

func assertChatWorkspaceEvent(
	t *testing.T,
	events <-chan workspacehub.Event,
	lifecycleEvent applicationlifecycle.ChatEvent,
) {
	t.Helper()
	select {
	case event := <-events:
		if lifecycleEvent.State == applicationlifecycle.ChatDeleted {
			if event.Type != "chat.delete" || event.ID != lifecycleEvent.ChatID {
				t.Fatalf("workspace event = %+v before lifecycle event %+v", event, lifecycleEvent)
			}
			return
		}
		if event.Type != "chat.upsert" || event.Chat == nil || string(event.Chat.ID) != lifecycleEvent.ChatID {
			t.Fatalf("workspace event = %+v before lifecycle event %+v", event, lifecycleEvent)
		}
	default:
		t.Fatalf("workspace event missing before lifecycle event %+v", lifecycleEvent)
	}
}

func TestNotifyingChatRepositorySkipsFailedRecordLifecycle(t *testing.T) {
	publisher := applicationlifecycle.NewChatPublisher()
	subscriber := &recordingChatLifecycleSubscriber{}
	publisher.Subscribe(subscriber)
	wantErr := errors.New("write failed")
	repository := notifyingChatRepository{
		Repository: lifecycleChatRepositoryStub{err: wantErr},
		workspace:  workspacehub.New(),
		lifecycle:  publisher,
	}
	ctx := context.Background()

	if _, err := repository.Create(ctx, servicechat.Meta{}); !errors.Is(err, wantErr) {
		t.Fatalf("Create() error = %v, want %v", err, wantErr)
	}
	if _, err := repository.Update(ctx, "cafebabe", func(*servicechat.Meta) {}); !errors.Is(err, wantErr) {
		t.Fatalf("Update() error = %v, want %v", err, wantErr)
	}
	if err := repository.Delete(ctx, "0123abcd"); !errors.Is(err, wantErr) {
		t.Fatalf("Delete() error = %v, want %v", err, wantErr)
	}
	if len(subscriber.events) != 0 {
		t.Fatalf("events = %+v, want none", subscriber.events)
	}
}

func TestNotifyingChatRepositorySkipsRecordLifecycleForTranscriptMutations(t *testing.T) {
	publisher := applicationlifecycle.NewChatPublisher()
	subscriber := &recordingChatLifecycleSubscriber{}
	publisher.Subscribe(subscriber)
	repository := notifyingChatRepository{
		Repository: lifecycleChatRepositoryStub{
			getResult:      servicechat.Meta{ID: "deadbeef"},
			truncateResult: []servicechat.Event{{Seq: 3, Type: "user"}},
		},
		workspace: workspacehub.New(),
		lifecycle: publisher,
	}
	ctx := context.Background()

	appended, err := repository.AppendEvent(ctx, "deadbeef", servicechat.Event{Type: "user"})
	if err != nil {
		t.Fatal(err)
	}
	if appended.Seq != 7 || appended.Type != "user" {
		t.Fatalf("AppendEvent() = %+v, want persisted user event", appended)
	}

	copied, err := repository.AppendCopiedEvent(ctx, "deadbeef", servicechat.Event{Type: "complete"})
	if err != nil {
		t.Fatal(err)
	}
	if copied.Seq != 7 || copied.Type != "complete" {
		t.Fatalf("AppendCopiedEvent() = %+v, want persisted complete event", copied)
	}

	truncated, err := repository.TruncateEventsBefore(ctx, "deadbeef", 4)
	if err != nil {
		t.Fatal(err)
	}
	if len(truncated) != 1 || truncated[0].Seq != 3 {
		t.Fatalf("TruncateEventsBefore() = %+v, want configured result", truncated)
	}
	if len(subscriber.events) != 0 {
		t.Fatalf("events = %+v, want none", subscriber.events)
	}
}

func assertChatLifecycleEvents(
	t *testing.T,
	got []recordedChatLifecycleEvent,
	ctx context.Context,
	want []applicationlifecycle.ChatEvent,
) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("events = %d, want %d", len(got), len(want))
	}
	for index := range want {
		if got[index].event != want[index] {
			t.Fatalf("event %d = %+v, want %+v", index, got[index].event, want[index])
		}
		if got[index].ctx != ctx {
			t.Fatalf("event %d did not receive the repository context", index)
		}
	}
}

type lifecycleProjectRepositoryStub struct {
	serviceproject.Repository
	createResult    serviceproject.Meta
	updateResult    serviceproject.Meta
	setStatusResult serviceproject.Meta
	err             error
}

func (r lifecycleProjectRepositoryStub) Create(
	context.Context,
	serviceproject.Meta,
) (serviceproject.Meta, error) {
	return r.createResult, r.err
}

func (r lifecycleProjectRepositoryStub) Update(
	context.Context,
	serviceproject.ID,
	func(*serviceproject.Meta),
) (serviceproject.Meta, error) {
	return r.updateResult, r.err
}

func (r lifecycleProjectRepositoryStub) SetStatus(
	context.Context,
	serviceproject.ID,
	serviceproject.Status,
	string,
) (serviceproject.Meta, error) {
	return r.setStatusResult, r.err
}

func (r lifecycleProjectRepositoryStub) Delete(context.Context, serviceproject.ID) error {
	return r.err
}

type recordedProjectLifecycleEvent struct {
	ctx   context.Context
	event applicationlifecycle.ProjectEvent
}

type recordingProjectLifecycleSubscriber struct {
	beforeRecord func(applicationlifecycle.ProjectEvent)
	events       []recordedProjectLifecycleEvent
}

func (s *recordingProjectLifecycleSubscriber) OnProject(
	ctx context.Context,
	event applicationlifecycle.ProjectEvent,
) {
	if s.beforeRecord != nil {
		s.beforeRecord(event)
	}
	s.events = append(s.events, recordedProjectLifecycleEvent{ctx: ctx, event: event})
}

func TestNotifyingProjectRepositoryPublishesSuccessfulRecordLifecycle(t *testing.T) {
	publisher := applicationlifecycle.NewProjectPublisher()
	workspace := workspacehub.New()
	workspaceEvents := workspace.Subscribe()
	defer workspaceEvents.Close()
	subscriber := &recordingProjectLifecycleSubscriber{
		beforeRecord: func(event applicationlifecycle.ProjectEvent) {
			assertProjectWorkspaceEvent(t, workspaceEvents.Events(), event)
		},
	}
	publisher.Subscribe(subscriber)
	repository := notifyingProjectRepository{
		Repository: lifecycleProjectRepositoryStub{
			createResult:    serviceproject.Meta{ID: "deadbeef"},
			updateResult:    serviceproject.Meta{ID: "cafebabe"},
			setStatusResult: serviceproject.Meta{ID: "0123abcd"},
		},
		workspace: workspace,
		lifecycle: publisher,
	}
	ctx := context.Background()

	if _, err := repository.Create(ctx, serviceproject.Meta{}); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.Update(ctx, "cafebabe", func(*serviceproject.Meta) {}); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.SetStatus(ctx, "0123abcd", serviceproject.StatusRunning, ""); err != nil {
		t.Fatal(err)
	}
	if err := repository.Delete(ctx, "feedface"); err != nil {
		t.Fatal(err)
	}

	want := []applicationlifecycle.ProjectEvent{
		{State: applicationlifecycle.ProjectCreated, ProjectID: "deadbeef"},
		{State: applicationlifecycle.ProjectUpdated, ProjectID: "cafebabe"},
		{State: applicationlifecycle.ProjectUpdated, ProjectID: "0123abcd"},
		{State: applicationlifecycle.ProjectDeleted, ProjectID: "feedface"},
	}
	assertProjectLifecycleEvents(t, subscriber.events, ctx, want)
}

func assertProjectWorkspaceEvent(
	t *testing.T,
	events <-chan workspacehub.Event,
	lifecycleEvent applicationlifecycle.ProjectEvent,
) {
	t.Helper()
	select {
	case event := <-events:
		if lifecycleEvent.State == applicationlifecycle.ProjectDeleted {
			if event.Type != "project.delete" || event.ID != lifecycleEvent.ProjectID {
				t.Fatalf("workspace event = %+v before lifecycle event %+v", event, lifecycleEvent)
			}
			return
		}
		if event.Type != "project.upsert" || event.Project == nil || string(event.Project.ID) != lifecycleEvent.ProjectID {
			t.Fatalf("workspace event = %+v before lifecycle event %+v", event, lifecycleEvent)
		}
	default:
		t.Fatalf("workspace event missing before lifecycle event %+v", lifecycleEvent)
	}
}

func TestNotifyingProjectRepositorySkipsFailedRecordLifecycle(t *testing.T) {
	publisher := applicationlifecycle.NewProjectPublisher()
	subscriber := &recordingProjectLifecycleSubscriber{}
	publisher.Subscribe(subscriber)
	wantErr := errors.New("write failed")
	repository := notifyingProjectRepository{
		Repository: lifecycleProjectRepositoryStub{err: wantErr},
		workspace:  workspacehub.New(),
		lifecycle:  publisher,
	}
	ctx := context.Background()

	if _, err := repository.Create(ctx, serviceproject.Meta{}); !errors.Is(err, wantErr) {
		t.Fatalf("Create() error = %v, want %v", err, wantErr)
	}
	if _, err := repository.Update(ctx, "cafebabe", func(*serviceproject.Meta) {}); !errors.Is(err, wantErr) {
		t.Fatalf("Update() error = %v, want %v", err, wantErr)
	}
	if _, err := repository.SetStatus(ctx, "0123abcd", serviceproject.StatusRunning, ""); !errors.Is(err, wantErr) {
		t.Fatalf("SetStatus() error = %v, want %v", err, wantErr)
	}
	if err := repository.Delete(ctx, "feedface"); !errors.Is(err, wantErr) {
		t.Fatalf("Delete() error = %v, want %v", err, wantErr)
	}
	if len(subscriber.events) != 0 {
		t.Fatalf("events = %+v, want none", subscriber.events)
	}
}

func assertProjectLifecycleEvents(
	t *testing.T,
	got []recordedProjectLifecycleEvent,
	ctx context.Context,
	want []applicationlifecycle.ProjectEvent,
) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("events = %d, want %d", len(got), len(want))
	}
	for index := range want {
		if got[index].event != want[index] {
			t.Fatalf("event %d = %+v, want %+v", index, got[index].event, want[index])
		}
		if got[index].ctx != ctx {
			t.Fatalf("event %d did not receive the repository context", index)
		}
	}
}
