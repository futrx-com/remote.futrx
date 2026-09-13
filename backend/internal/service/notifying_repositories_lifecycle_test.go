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
	createResult servicechat.Meta
	updateResult servicechat.Meta
	err          error
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

type recordedChatLifecycleEvent struct {
	ctx   context.Context
	event applicationlifecycle.ChatEvent
}

type recordingChatLifecycleSubscriber struct {
	events []recordedChatLifecycleEvent
}

func (s *recordingChatLifecycleSubscriber) OnChat(
	ctx context.Context,
	event applicationlifecycle.ChatEvent,
) {
	s.events = append(s.events, recordedChatLifecycleEvent{ctx: ctx, event: event})
}

func TestNotifyingChatRepositoryPublishesSuccessfulRecordLifecycle(t *testing.T) {
	publisher := applicationlifecycle.NewChatPublisher()
	subscriber := &recordingChatLifecycleSubscriber{}
	publisher.Subscribe(subscriber)
	repository := notifyingChatRepository{
		Repository: lifecycleChatRepositoryStub{
			createResult: servicechat.Meta{ID: "deadbeef"},
			updateResult: servicechat.Meta{ID: "cafebabe"},
		},
		workspace: workspacehub.New(),
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
	events []recordedProjectLifecycleEvent
}

func (s *recordingProjectLifecycleSubscriber) OnProject(
	ctx context.Context,
	event applicationlifecycle.ProjectEvent,
) {
	s.events = append(s.events, recordedProjectLifecycleEvent{ctx: ctx, event: event})
}

func TestNotifyingProjectRepositoryPublishesSuccessfulRecordLifecycle(t *testing.T) {
	publisher := applicationlifecycle.NewProjectPublisher()
	subscriber := &recordingProjectLifecycleSubscriber{}
	publisher.Subscribe(subscriber)
	repository := notifyingProjectRepository{
		Repository: lifecycleProjectRepositoryStub{
			createResult:    serviceproject.Meta{ID: "deadbeef"},
			updateResult:    serviceproject.Meta{ID: "cafebabe"},
			setStatusResult: serviceproject.Meta{ID: "0123abcd"},
		},
		workspace: workspacehub.New(),
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
