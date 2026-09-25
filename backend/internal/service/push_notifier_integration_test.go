package service

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
	servicepresence "github.com/futrx-com/remote.futrx.com/internal/service/presence"
	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
	servicepush "github.com/futrx-com/remote.futrx.com/internal/service/push"
	serviceuser "github.com/futrx-com/remote.futrx.com/internal/service/user"
	"github.com/futrx-com/remote.futrx.com/internal/service/workspacehub"
)

// chatRepoStub is the minimum of servicechat.Repository the append path
// touches: store one chat's metadata, and echo appended events back.
type chatRepoStub struct {
	mu   sync.Mutex
	meta servicechat.Meta
	seq  int64
}

func (r *chatRepoStub) List(context.Context) ([]servicechat.Meta, error) {
	return []servicechat.Meta{r.meta}, nil
}

func (r *chatRepoStub) Create(_ context.Context, meta servicechat.Meta) (servicechat.Meta, error) {
	return meta, nil
}

func (r *chatRepoStub) Get(context.Context, servicechat.ID) (servicechat.Meta, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.meta, nil
}

func (r *chatRepoStub) Update(
	_ context.Context,
	_ servicechat.ID,
	fn func(*servicechat.Meta),
) (servicechat.Meta, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	fn(&r.meta)
	return r.meta, nil
}

func (r *chatRepoStub) Delete(context.Context, servicechat.ID) error { return nil }

func (r *chatRepoStub) ReadEvents(context.Context, servicechat.ID) ([]servicechat.Event, error) {
	return nil, nil
}

func (r *chatRepoStub) ReadEventsPage(
	context.Context,
	servicechat.ID,
	servicechat.EventPageQuery,
) (servicechat.EventPage, error) {
	return servicechat.EventPage{}, nil
}

func (r *chatRepoStub) ReadEventsAfter(
	context.Context,
	servicechat.ID,
	int64,
) ([]servicechat.Event, error) {
	return nil, nil
}

func (r *chatRepoStub) AppendEvent(
	_ context.Context,
	_ servicechat.ID,
	ev servicechat.Event,
) (servicechat.Event, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seq++
	ev.Seq = r.seq
	return ev, nil
}

func (r *chatRepoStub) TruncateEventsBefore(
	context.Context,
	servicechat.ID,
	int64,
) ([]servicechat.Event, error) {
	return nil, nil
}

type userRepoStub struct {
	users []serviceuser.User
}

func (r userRepoStub) List(context.Context) ([]serviceuser.User, error) { return r.users, nil }

func (r userRepoStub) Get(_ context.Context, email string) (*serviceuser.User, error) {
	for i := range r.users {
		if r.users[i].Email == email {
			return &r.users[i], nil
		}
	}
	return nil, nil
}

func (r userRepoStub) Add(context.Context, serviceuser.User) error             { return nil }
func (r userRepoStub) Remove(context.Context, string) error                    { return nil }
func (r userRepoStub) SetRole(context.Context, string, serviceuser.Role) error { return nil }
func (r userRepoStub) Count(context.Context) (int, error)                      { return len(r.users), nil }

type capturingSender struct {
	mu       sync.Mutex
	payloads []servicepush.Notification
	// fail, when set, rejects every delivery with this error.
	fail error
}

func (s *capturingSender) failWith(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fail = err
}

func (s *capturingSender) PublicKey() string { return "BTestKey" }

func (s *capturingSender) Send(
	_ context.Context,
	_ servicepush.Subscription,
	payload []byte,
	_ bool,
) error {
	var notification servicepush.Notification
	if err := json.Unmarshal(payload, &notification); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fail != nil {
		return s.fail
	}
	s.payloads = append(s.payloads, notification)
	return nil
}

func (s *capturingSender) captured() []servicepush.Notification {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]servicepush.Notification(nil), s.payloads...)
}

type pushRepoStub struct {
	mu   sync.Mutex
	rows map[string][]servicepush.Subscription
}

type notificationProjectStub struct{ name string }

func (s notificationProjectStub) Get(_ context.Context, id serviceproject.ID) (serviceproject.Meta, error) {
	return serviceproject.Meta{ID: id, Name: s.name}, nil
}

func (s notificationProjectStub) ListAccess(_ context.Context, _ serviceproject.ID) ([]string, error) {
	return []string{"owner@example.com"}, nil
}

func (r *pushRepoStub) List(_ context.Context, email string) ([]servicepush.Subscription, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]servicepush.Subscription(nil), r.rows[email]...), nil
}

func (r *pushRepoStub) Save(_ context.Context, email string, sub servicepush.Subscription) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for index, existing := range r.rows[email] {
		if existing.Endpoint == sub.Endpoint {
			r.rows[email][index] = sub
			return nil
		}
	}
	r.rows[email] = append(r.rows[email], sub)
	return nil
}

func (r *pushRepoStub) Delete(_ context.Context, email, endpoint string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	kept := r.rows[email][:0]
	for _, sub := range r.rows[email] {
		if sub.Endpoint != endpoint {
			kept = append(kept, sub)
		}
	}
	r.rows[email] = kept
	return nil
}

func (r *pushRepoStub) DeleteAll(_ context.Context, email string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.rows, email)
	return nil
}

// newNotifyingChat assembles the same wrapper services.New builds, so the test
// exercises the real append -> notify path rather than the notifier alone.
func newNotifyingChat(t *testing.T, meta servicechat.Meta) (
	notifyingChatRepository,
	*capturingSender,
) {
	t.Helper()

	pushRepo := &pushRepoStub{rows: map[string][]servicepush.Subscription{}}
	sender := &capturingSender{}
	pushService := servicepush.New(pushRepo, sender)

	subscription := servicepush.Subscription{
		Endpoint:  "https://push.example.com/device-1",
		P256dh:    "BGsX0fLhLEJH-Lzm5WOkQPJ3A32BLeszoPShOUXYmMKWT-NC4v4af5uO5-tKfA-eFivOM1drMV7Oy7ZAaDe_UfU",
		Auth:      "AAAAAAAAAAAAAAAAAAAAAA",
		CreatedAt: 1,
	}
	if err := pushService.Subscribe(context.Background(), "owner@example.com", subscription); err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	chats := &chatRepoStub{meta: meta}
	users := serviceuser.New(userRepoStub{users: []serviceuser.User{
		{Email: "owner@example.com", Role: serviceuser.RoleAdmin},
	}})

	return notifyingChatRepository{
		Repository: chats,
		workspace:  workspacehub.New(),
		push: &chatPushNotifier{
			push:     pushService,
			chats:    chats,
			audience: chatNotificationAudience{users: users},
			presence: servicepresence.New(),
		},
	}, sender
}

func TestAppendingATerminalEventRaisesANotification(t *testing.T) {
	repo, sender := newNotifyingChat(t, servicechat.Meta{
		ID:    "beefcafe",
		Title: "Fix the flaky upload test",
	})

	_, err := repo.AppendEvent(context.Background(), "beefcafe", servicechat.Event{
		T:    1,
		Type: "complete",
	})
	if err != nil {
		t.Fatalf("append: %v", err)
	}
	repo.push.push.Wait()

	sent := sender.captured()
	if len(sent) != 1 {
		t.Fatalf("captured %d notifications, want 1", len(sent))
	}
	if sent[0].Title != "Remote - Agent finished" || sent[0].Body != "Open the chat to see the result." {
		t.Fatalf("notification = %+v", sent[0])
	}
	if sent[0].ChatID != "beefcafe" {
		t.Fatalf("chat id = %q", sent[0].ChatID)
	}
	// The tag is what lets a busy chat replace its own tray entry instead of
	// stacking a new one per turn.
	if sent[0].Tag != "chat:beefcafe" {
		t.Fatalf("tag = %q", sent[0].Tag)
	}
}

func TestProjectCompletionUsesProjectNameAndPrivateSummary(t *testing.T) {
	repo, sender := newNotifyingChat(t, servicechat.Meta{
		ID: "abcdef12", ProjectID: "aabbccdd", Title: "An unrelated chat title",
	})
	projects := notificationProjectStub{name: "Website"}
	repo.push.projects = projects
	repo.push.audience.projects = projects
	_, err := repo.AppendEvent(context.Background(), "abcdef12", servicechat.Event{
		Type: "complete", NotificationSummary: "Fixed settings refresh and chat links.",
	})
	if err != nil {
		t.Fatal(err)
	}
	repo.push.push.Wait()
	sent := sender.captured()
	if len(sent) != 1 || sent[0].Title != "Website - Agent finished" || sent[0].Body != "Fixed settings refresh and chat links." || sent[0].ChatID != "abcdef12" {
		t.Fatalf("notification = %+v", sent)
	}
}

func TestAppendingCopiedHistoryDoesNotRaiseNotifications(t *testing.T) {
	repo, sender := newNotifyingChat(t, servicechat.Meta{ID: "beefcafe", Title: "Forked chat"})
	ctx := context.Background()

	for _, event := range []servicechat.Event{
		{Type: "tool_use_start", Name: "AskUserQuestion"},
		{Type: "complete"},
		{Type: "error", Message: "historical failure"},
	} {
		if _, err := repo.AppendCopiedEvent(ctx, "beefcafe", event); err != nil {
			t.Fatal(err)
		}
	}
	repo.push.push.Wait()

	if sent := sender.captured(); len(sent) != 0 {
		t.Fatalf("historical replay sent notifications: %+v", sent)
	}
}

func TestProjectAudienceExcludesAccessEntriesForRemovedUsers(t *testing.T) {
	projects := []serviceproject.Meta{{ID: "beefcafe"}}
	access := &cleanupProjectAccess{members: map[serviceproject.ID][]string{
		"beefcafe": {"active@example.com", "removed@example.com"},
	}}
	audience := chatNotificationAudience{
		projects: serviceproject.New(
			cleanupProjectRepository{projects: projects},
			serviceproject.ContainerDependencies{},
			nil,
			access,
		),
		users: serviceuser.New(userRepoStub{users: []serviceuser.User{
			{Email: "active@example.com", Role: serviceuser.RoleMember},
			{Email: "admin@example.com", Role: serviceuser.RoleAdmin},
		}}),
	}

	recipients, err := audience.recipients(context.Background(), servicechat.Meta{ProjectID: "beefcafe"})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"active@example.com": true, "admin@example.com": true}
	if len(recipients) != len(want) {
		t.Fatalf("recipients = %v, want active member and admin", recipients)
	}
	for _, recipient := range recipients {
		if !want[recipient] {
			t.Fatalf("unexpected recipient %q in %v", recipient, recipients)
		}
	}
}

func TestAppendingAQuestionRaisesAnUrgentNotification(t *testing.T) {
	repo, sender := newNotifyingChat(t, servicechat.Meta{ID: "beefcafe", Title: "Plan the migration"})

	_, err := repo.AppendEvent(context.Background(), "beefcafe", servicechat.Event{
		T:    1,
		Type: "tool_use_start",
		ID:   "tool-1",
		Name: "AskUserQuestion",
	})
	if err != nil {
		t.Fatal(err)
	}
	repo.push.push.Wait()

	sent := sender.captured()
	if len(sent) != 1 {
		t.Fatalf("captured %d notifications, want 1", len(sent))
	}
	if sent[0].Kind != servicepush.KindQuestion {
		t.Fatalf("kind = %q", sent[0].Kind)
	}
}

func TestStreamingEventsRaiseNothing(t *testing.T) {
	repo, sender := newNotifyingChat(t, servicechat.Meta{ID: "beefcafe", Title: "Chat"})
	ctx := context.Background()

	for _, event := range []servicechat.Event{
		{Type: "user", Text: "go"},
		{Type: "assistant_text", Text: "working"},
		{Type: "thinking", Text: "hmm"},
		{Type: "tool_use_start", Name: "Bash"},
		{Type: "tool_use_end", ID: "tool-1"},
		{Type: "session"},
	} {
		if _, err := repo.AppendEvent(ctx, "beefcafe", event); err != nil {
			t.Fatal(err)
		}
	}
	repo.push.push.Wait()

	if sent := sender.captured(); len(sent) != 0 {
		t.Fatalf("captured %+v, want nothing", sent)
	}
}

func TestScheduledRunsAreLabelledSeparately(t *testing.T) {
	repo, sender := newNotifyingChat(t, servicechat.Meta{ID: "beefcafe", Title: "Nightly audit"})

	_, err := repo.AppendEvent(context.Background(), "beefcafe", servicechat.Event{
		T:               1,
		Type:            "complete",
		ScheduledTaskID: "task-7",
	})
	if err != nil {
		t.Fatal(err)
	}
	repo.push.push.Wait()

	sent := sender.captured()
	if len(sent) != 1 || sent[0].Kind != servicepush.KindScheduled {
		t.Fatalf("notifications = %+v", sent)
	}
	if sent[0].Title != "Remote - Agent finished" || sent[0].Body != "A scheduled task finished." {
		t.Fatalf("body = %q", sent[0].Body)
	}
}

func TestAQuestionSuppressesTheTurnFinishedThatFollowsIt(t *testing.T) {
	repo, sender := newNotifyingChat(t, servicechat.Meta{ID: "beefcafe", Title: "Plan the migration"})
	ctx := context.Background()

	// The agent asks, then the run ends immediately — the answer arrives as a
	// fresh prompt, not a tool result.
	_, _ = repo.AppendEvent(ctx, "beefcafe", servicechat.Event{Type: "tool_use_start", Name: "AskUserQuestion"})
	_, _ = repo.AppendEvent(ctx, "beefcafe", servicechat.Event{Type: "complete"})
	repo.push.push.Wait()

	sent := sender.captured()
	if len(sent) != 1 {
		t.Fatalf("captured %+v, want only the question", sent)
	}
	if sent[0].Kind != servicepush.KindQuestion {
		t.Fatalf("kind = %q", sent[0].Kind)
	}

	// Answering starts a new run, whose completion notifies normally again.
	_, _ = repo.AppendEvent(ctx, "beefcafe", servicechat.Event{Type: "user", Text: "the second option"})
	_, _ = repo.AppendEvent(ctx, "beefcafe", servicechat.Event{Type: "complete"})
	repo.push.push.Wait()

	sent = sender.captured()
	if len(sent) != 2 || sent[1].Kind != servicepush.KindComplete {
		t.Fatalf("captured %+v, want the follow-up completion", sent)
	}
}

func TestASecondRunAfterAnAbandonedQuestionStillNotifies(t *testing.T) {
	repo, sender := newNotifyingChat(t, servicechat.Meta{ID: "beefcafe", Title: "Plan"})
	ctx := context.Background()

	_, _ = repo.AppendEvent(ctx, "beefcafe", servicechat.Event{Type: "tool_use_start", Name: "AskUserQuestion"})
	_, _ = repo.AppendEvent(ctx, "beefcafe", servicechat.Event{Type: "complete"})
	repo.push.push.Wait()

	// A run that fails outright later must not stay swallowed by the stale
	// parked flag.
	_, _ = repo.AppendEvent(ctx, "beefcafe", servicechat.Event{Type: "user", Text: "never mind"})
	_, _ = repo.AppendEvent(ctx, "beefcafe", servicechat.Event{Type: "error", Message: "boom"})
	repo.push.push.Wait()

	sent := sender.captured()
	if len(sent) != 2 || sent[1].Kind != servicepush.KindError {
		t.Fatalf("captured %+v", sent)
	}
}

// The reason presence exists: the service worker can only silence the browser
// it runs in, so without a server-side signal the owner's other phone buzzes
// about a question they are reading right now.
func TestWatchingAChatSilencesEveryDeviceTheUserOwns(t *testing.T) {
	repo, sender := newNotifyingChat(t, servicechat.Meta{ID: "beefcafe", Title: "Plan"})
	repo.push.presence.Record("owner@example.com", servicepresence.Report{
		ClientID: "laptop",
		ChatID:   "beefcafe",
		Revision: 1,
	})

	_, err := repo.AppendEvent(context.Background(), "beefcafe", servicechat.Event{
		Type: "tool_use_start",
		Name: "AskUserQuestion",
	})
	if err != nil {
		t.Fatal(err)
	}
	repo.push.push.Wait()

	if sent := sender.captured(); len(sent) != 0 {
		t.Fatalf("captured %+v, want nothing while the user is watching", sent)
	}
}

func TestWatchingADifferentChatStillNotifies(t *testing.T) {
	repo, sender := newNotifyingChat(t, servicechat.Meta{ID: "beefcafe", Title: "Plan"})
	// Reading one chat is no reason to miss another agent needing an answer.
	repo.push.presence.Record("owner@example.com", servicepresence.Report{
		ClientID: "laptop",
		ChatID:   "otherchat",
		Revision: 1,
	})

	_, err := repo.AppendEvent(context.Background(), "beefcafe", servicechat.Event{Type: "complete"})
	if err != nil {
		t.Fatal(err)
	}
	repo.push.push.Wait()

	if sent := sender.captured(); len(sent) != 1 {
		t.Fatalf("captured %d notifications, want 1", len(sent))
	}
}

func TestLeavingAChatRestoresNotifications(t *testing.T) {
	repo, sender := newNotifyingChat(t, servicechat.Meta{ID: "beefcafe", Title: "Plan"})
	ctx := context.Background()

	repo.push.presence.Record("owner@example.com", servicepresence.Report{
		ClientID: "laptop",
		ChatID:   "beefcafe",
		Revision: 1,
	})
	_, _ = repo.AppendEvent(ctx, "beefcafe", servicechat.Event{Type: "complete"})
	repo.push.push.Wait()

	// Backgrounding the tab withdraws the claim, and the next turn lands.
	repo.push.presence.Record("owner@example.com", servicepresence.Report{
		ClientID: "laptop",
		Revision: 2,
	})
	_, _ = repo.AppendEvent(ctx, "beefcafe", servicechat.Event{Type: "complete"})
	repo.push.push.Wait()

	if sent := sender.captured(); len(sent) != 1 {
		t.Fatalf("captured %+v, want only the notification raised after leaving", sent)
	}
}

// iOS ignores the per-chat tag and stacks every notification, so an unread
// chat raises one notification and then stays quiet until it is read.
func TestAnUnreadChatDoesNotNotifyAgainUntilItIsRead(t *testing.T) {
	repo, sender := newNotifyingChat(t, servicechat.Meta{ID: "beefcafe", Title: "Plan"})
	ctx := context.Background()
	chats := repo.Repository.(*chatRepoStub)
	appendAt := func(ev servicechat.Event) {
		t.Helper()
		if _, err := repo.AppendEvent(ctx, "beefcafe", ev); err != nil {
			t.Fatal(err)
		}
		repo.push.push.Wait()
	}

	appendAt(servicechat.Event{T: 10, Type: "complete"})
	appendAt(servicechat.Event{T: 20, Type: "error", Message: "boom"})
	scheduled := servicechat.Event{T: 30, Type: "user", Text: "nightly", ScheduledTaskID: "task1"}
	appendAt(scheduled)
	appendAt(servicechat.Event{T: 40, Type: "complete", ScheduledTaskID: "task1"})
	if sent := sender.captured(); len(sent) != 1 {
		t.Fatalf("captured %+v, want only the first notification", sent)
	}

	// A question still gets through: the run is blocked on the user.
	appendAt(servicechat.Event{T: 50, Type: "tool_use_start", Name: "AskUserQuestion"})
	if sent := sender.captured(); len(sent) != 2 || sent[1].Kind != servicepush.KindQuestion {
		t.Fatalf("captured %+v, want the question as well", sent)
	}

	// The next scheduled run starts; its completion would still stay quiet.
	appendAt(servicechat.Event{T: 55, Type: "user", Text: "nightly", ScheduledTaskID: "task1"})
	// Reading the chat reopens it for the next notification.
	_, _ = chats.Update(ctx, "beefcafe", func(m *servicechat.Meta) { m.LastReadAt = 55 })
	appendAt(servicechat.Event{T: 60, Type: "complete", ScheduledTaskID: "task1"})
	if sent := sender.captured(); len(sent) != 3 {
		t.Fatalf("captured %+v, want a notification after the read", sent)
	}
}

// A question marks the chat as notified too, so a scheduled run that finishes
// after it must not stack a second entry before the user has read the chat.
func TestAScheduledFinishAfterAQuestionStaysQuietUntilRead(t *testing.T) {
	repo, sender := newNotifyingChat(t, servicechat.Meta{ID: "beefcafe", Title: "Plan"})
	ctx := context.Background()
	chats := repo.Repository.(*chatRepoStub)
	appendEvent := func(ev servicechat.Event) {
		t.Helper()
		if _, err := repo.AppendEvent(ctx, "beefcafe", ev); err != nil {
			t.Fatal(err)
		}
		repo.push.push.Wait()
	}

	appendEvent(servicechat.Event{T: 10, Type: "tool_use_start", Name: "AskUserQuestion"})
	appendEvent(servicechat.Event{T: 20, Type: "complete"})
	appendEvent(servicechat.Event{T: 30, Type: "user", Text: "nightly", ScheduledTaskID: "task1"})
	appendEvent(servicechat.Event{T: 40, Type: "complete", ScheduledTaskID: "task1"})
	if sent := sender.captured(); len(sent) != 1 || sent[0].Kind != servicepush.KindQuestion {
		t.Fatalf("captured %+v, want only the question", sent)
	}

	_, _ = chats.Update(ctx, "beefcafe", func(m *servicechat.Meta) { m.LastReadAt = 45 })
	appendEvent(servicechat.Event{T: 50, Type: "user", Text: "nightly", ScheduledTaskID: "task1"})
	appendEvent(servicechat.Event{T: 60, Type: "complete", ScheduledTaskID: "task1"})
	if sent := sender.captured(); len(sent) != 2 || sent[1].Kind != servicepush.KindScheduled {
		t.Fatalf("captured %+v, want the scheduled finish after the read", sent)
	}
}

// Suppression exists to avoid repeating a notification the user already has.
// One that never reached a device must not silence the chat.
func TestAFailedDeliveryDoesNotSilenceTheChat(t *testing.T) {
	repo, sender := newNotifyingChat(t, servicechat.Meta{ID: "beefcafe", Title: "Plan"})
	ctx := context.Background()

	sender.failWith(errors.New("503 service unavailable"))
	_, _ = repo.AppendEvent(ctx, "beefcafe", servicechat.Event{T: 10, Type: "complete"})
	repo.push.push.Wait()
	if sent := sender.captured(); len(sent) != 0 {
		t.Fatalf("captured %+v, want nothing while delivery fails", sent)
	}

	sender.failWith(nil)
	_, _ = repo.AppendEvent(ctx, "beefcafe", servicechat.Event{T: 20, Type: "error", Message: "boom"})
	repo.push.push.Wait()
	if sent := sender.captured(); len(sent) != 1 || sent[0].Kind != servicepush.KindError {
		t.Fatalf("captured %+v, want the next event to notify", sent)
	}
}

func TestDeletingAChatForgetsItsNotificationState(t *testing.T) {
	repo, _ := newNotifyingChat(t, servicechat.Meta{ID: "beefcafe", Title: "Plan"})
	ctx := context.Background()

	_, _ = repo.AppendEvent(ctx, "beefcafe", servicechat.Event{Type: "tool_use_start", Name: "AskUserQuestion"})
	repo.push.push.Wait()
	if err := repo.Delete(ctx, "beefcafe"); err != nil {
		t.Fatal(err)
	}

	repo.push.mu.Lock()
	defer repo.push.mu.Unlock()
	if len(repo.push.parked) != 0 || len(repo.push.notified) != 0 {
		t.Fatalf("parked = %v, notified = %v; want both empty", repo.push.parked, repo.push.notified)
	}
}
