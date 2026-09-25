package applications

import (
	"context"
	"sync"
	"testing"
	"time"

	applicationapi "github.com/futrx-com/remote.futrx.com/pkg/applications"
)

type operationGate struct {
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func newOperationGate() *operationGate {
	return &operationGate{entered: make(chan struct{}), release: make(chan struct{})}
}

func (g *operationGate) wait(ctx context.Context) error {
	if g == nil {
		return nil
	}
	g.once.Do(func() { close(g.entered) })
	select {
	case <-g.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

type serializationInstaller struct {
	install   *operationGate
	stop      *operationGate
	uninstall *operationGate
}

func (i *serializationInstaller) Install(ctx context.Context, _ InstallSpec) error {
	return i.install.wait(ctx)
}

func (*serializationInstaller) Start(context.Context, InstallSpec) error { return nil }

func (i *serializationInstaller) Stop(ctx context.Context, _ InstallSpec) error {
	return i.stop.wait(ctx)
}

func (i *serializationInstaller) Uninstall(ctx context.Context, _ InstallSpec) error {
	return i.uninstall.wait(ctx)
}

func (*serializationInstaller) Expose(context.Context, InstallSpec) error { return nil }

type serializationHost struct {
	call     *operationGate
	notified chan applicationapi.Instance
}

func (h *serializationHost) Ensure(
	context.Context,
	applicationapi.Instance,
) (applicationapi.Descriptor, error) {
	return applicationapi.Descriptor{APIVersion: applicationapi.APIVersion}, nil
}

func (h *serializationHost) Call(
	ctx context.Context,
	_ applicationapi.Instance,
	_ applicationapi.Request,
) (applicationapi.Response, error) {
	if err := h.call.wait(ctx); err != nil {
		return applicationapi.Response{}, err
	}
	return applicationapi.Response{Status: 200}, nil
}

func (h *serializationHost) Notify(
	_ context.Context,
	instance applicationapi.Instance,
	_ applicationapi.Event,
) error {
	h.notified <- instance
	return nil
}

func (*serializationHost) Stop(context.Context, string) error   { return nil }
func (*serializationHost) Remove(context.Context, string) error { return nil }
func (*serializationHost) InvalidateApplication(string)         {}

type blockingStoppedPublisher struct {
	*recordingApplicationLifecyclePublisher
	stoppedEntered chan struct{}
	releaseStopped chan struct{}
	once           sync.Once
}

func (p *blockingStoppedPublisher) PublishApplicationStopped(
	ctx context.Context,
	applicationID, instanceID, scope, projectID string,
) {
	p.recordingApplicationLifecyclePublisher.PublishApplicationStopped(
		ctx, applicationID, instanceID, scope, projectID)
	p.once.Do(func() { close(p.stoppedEntered) })
	<-p.releaseStopped
}

// observedGetStore exposes deterministic barriers around the router's stale
// ListAll snapshot and its later locked Get. The lifecycle operation performs
// the first Get; the router's recheck performs the second.
type observedGetStore struct {
	*fakeStore

	mu         sync.Mutex
	gets       int
	listed     chan struct{}
	reloaded   chan struct{}
	listedOnce sync.Once
	reloadOnce sync.Once
}

func newObservedGetStore(instance Instance) *observedGetStore {
	return &observedGetStore{
		fakeStore: &fakeStore{global: []Instance{instance}},
		listed:    make(chan struct{}),
		reloaded:  make(chan struct{}),
	}
}

func (s *observedGetStore) ListAll(ctx context.Context) ([]Instance, error) {
	instances, err := s.fakeStore.ListAll(ctx)
	s.listedOnce.Do(func() { close(s.listed) })
	return instances, err
}

func (s *observedGetStore) Get(ctx context.Context, id string) (Instance, bool, error) {
	instance, found, err := s.fakeStore.Get(ctx, id)
	s.mu.Lock()
	s.gets++
	if s.gets >= 2 {
		s.reloadOnce.Do(func() { close(s.reloaded) })
	}
	s.mu.Unlock()
	return instance, found, err
}

func TestEventRoutingWaitsForStopThenRechecksStoppedState(t *testing.T) {
	stop := newOperationGate()
	installer := &serializationInstaller{stop: stop}
	service, source, host, store, cancel := newSerializedRoutingFixture(
		t, installer, "1", "1", StatusRunning)
	defer cancel()

	stopped := make(chan error, 1)
	go func() {
		_, err := service.Stop(context.Background(), "copy-1")
		stopped <- err
	}()
	awaitSignal(t, stop.entered, "stop to enter installer")

	publishRoutingTestEvent(source)
	awaitSignal(t, store.listed, "router to capture candidates")
	assertNoSerializedNotification(t, host.notified)

	close(stop.release)
	if err := awaitError(t, stopped, "stop"); err != nil {
		t.Fatalf("stop: %v", err)
	}
	awaitSignal(t, store.reloaded, "router to reload stopped instance")
	assertNoSerializedNotification(t, host.notified)
}

func TestEventRoutingWaitsForUninstallThenSkipsDeletedInstance(t *testing.T) {
	uninstall := newOperationGate()
	installer := &serializationInstaller{uninstall: uninstall}
	service, source, host, store, cancel := newSerializedRoutingFixture(
		t, installer, "1", "1", StatusRunning)
	defer cancel()

	removed := make(chan error, 1)
	go func() { removed <- service.Uninstall(context.Background(), "copy-1") }()
	awaitSignal(t, uninstall.entered, "uninstall to enter installer")

	publishRoutingTestEvent(source)
	awaitSignal(t, store.listed, "router to capture candidates")
	assertNoSerializedNotification(t, host.notified)

	close(uninstall.release)
	if err := awaitError(t, removed, "uninstall"); err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	awaitSignal(t, store.reloaded, "router to reload deleted instance")
	assertNoSerializedNotification(t, host.notified)
}

func TestEventRoutingWaitsForUpgradeInstallBeforeNotify(t *testing.T) {
	install := newOperationGate()
	installer := &serializationInstaller{install: install}
	service, source, host, store, cancel := newSerializedRoutingFixture(
		t, installer, "2", "1", StatusRunning)
	defer cancel()

	upgraded := make(chan []UpgradeOutcome, 1)
	go func() {
		upgraded <- service.upgradeInstances(context.Background(), "consumer", []Instance{{
			ID: "copy-1", ApplicationID: "consumer", Status: StatusRunning,
		}})
	}()
	awaitSignal(t, install.entered, "upgrade to enter installer")

	publishRoutingTestEvent(source)
	awaitSignal(t, store.listed, "router to capture candidates")
	assertNoSerializedNotification(t, host.notified)

	close(install.release)
	select {
	case outcomes := <-upgraded:
		if len(outcomes) != 1 || outcomes[0].Error != "" {
			t.Fatalf("upgrade outcomes = %+v", outcomes)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for upgrade")
	}
	awaitSignal(t, store.reloaded, "router to reload upgraded instance")
	select {
	case instance := <-host.notified:
		if instance.ApplicationVersion != "2" {
			t.Fatalf("notified with application version %q, want 2", instance.ApplicationVersion)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for post-upgrade notification")
	}
}

func TestUpgradeReloadsAfterConcurrentStopAndLeavesInstanceStopped(t *testing.T) {
	stop := newOperationGate()
	install := newOperationGate()
	installer := &serializationInstaller{stop: stop, install: install}
	service, _, _, store, cancel := newSerializedRoutingFixture(
		t, installer, "2", "1", StatusRunning)
	defer cancel()

	stopped := make(chan error, 1)
	go func() {
		_, err := service.Stop(context.Background(), "copy-1")
		stopped <- err
	}()
	awaitSignal(t, stop.entered, "stop to enter installer")

	upgraded := make(chan []UpgradeOutcome, 1)
	go func() {
		upgraded <- service.upgradeInstances(context.Background(), "consumer", []Instance{{
			ID:                 "copy-1",
			ApplicationID:      "consumer",
			ApplicationVersion: "1",
			Status:             StatusRunning,
		}})
	}()
	close(stop.release)
	if err := awaitError(t, stopped, "stop"); err != nil {
		t.Fatalf("stop: %v", err)
	}
	select {
	case outcomes := <-upgraded:
		if len(outcomes) != 0 {
			t.Fatalf("upgrade outcomes = %+v, want none for stopped instance", outcomes)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for skipped upgrade")
	}
	awaitSignal(t, store.reloaded, "upgrade to reload stopped instance")
	assertNotSignaled(t, install.entered, "stale upgrade reinstalled a stopped instance")
}

func TestBackendCallReadLockKeepsStopOutOfLazyLaunch(t *testing.T) {
	call := newOperationGate()
	stop := newOperationGate()
	application := serializedSubscriberApplication("1")
	instance := serializedSubscriberInstance("1", StatusRunning)
	store := &fakeStore{global: []Instance{instance}}
	host := &serializationHost{call: call, notified: make(chan applicationapi.Instance, 1)}
	service := New(
		eventTestRegistry{"consumer": application},
		store,
		&serializationInstaller{stop: stop},
		nil,
		nil,
		WithBackendHost(host),
	)

	called := make(chan error, 1)
	go func() {
		_, err := service.CallBackend(
			context.Background(),
			"copy-1",
			applicationapi.Request{Path: "health"},
			applicationapi.Caller{Email: "user@example.com"},
		)
		called <- err
	}()
	awaitSignal(t, call.entered, "backend call to enter host")

	stopped := make(chan error, 1)
	go func() {
		_, err := service.Stop(context.Background(), "copy-1")
		stopped <- err
	}()
	assertNotSignaled(t, stop.entered, "stop entered installer while backend call held read lock")

	close(call.release)
	if err := awaitError(t, called, "backend call"); err != nil {
		t.Fatalf("backend call: %v", err)
	}
	awaitSignal(t, stop.entered, "stop after backend call")
	close(stop.release)
	if err := awaitError(t, stopped, "stop"); err != nil {
		t.Fatalf("stop: %v", err)
	}
}

func TestInstanceLifecycleEventsKeepCommittedTransitionOrder(t *testing.T) {
	application := serializedSubscriberApplication("1")
	store := &fakeStore{global: []Instance{serializedSubscriberInstance("1", StatusRunning)}}
	publisher := &blockingStoppedPublisher{
		recordingApplicationLifecyclePublisher: &recordingApplicationLifecyclePublisher{},
		stoppedEntered:                         make(chan struct{}),
		releaseStopped:                         make(chan struct{}),
	}
	service := New(
		eventTestRegistry{"consumer": application},
		store,
		&serializationInstaller{},
		nil,
		nil,
		WithBackendHost(&serializationHost{notified: make(chan applicationapi.Instance, 1)}),
		WithLifecyclePublisher(publisher),
	)

	stopped := make(chan error, 1)
	go func() {
		_, err := service.Stop(context.Background(), "copy-1")
		stopped <- err
	}()
	awaitSignal(t, publisher.stoppedEntered, "stopped event publication")

	started := make(chan error, 1)
	go func() {
		_, err := service.Start(context.Background(), "copy-1")
		started <- err
	}()
	if got := lifecycleEventKinds(publisher.events); !equalStrings(got, []string{"stopped"}) {
		t.Fatalf("events while stopped publication is blocked = %v", got)
	}

	close(publisher.releaseStopped)
	if err := awaitError(t, stopped, "stop"); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if err := awaitError(t, started, "start"); err != nil {
		t.Fatalf("start: %v", err)
	}
	if got := lifecycleEventKinds(publisher.events); !equalStrings(got, []string{"stopped", "started"}) {
		t.Fatalf("event order = %v, want [stopped started]", got)
	}
}

func TestInstanceLockSetReferenceCountsAndEvicts(t *testing.T) {
	var locks instanceLockSet
	first := locks.rlock("copy-1")
	second := locks.rlock("copy-1")

	locks.mu.Lock()
	entry := locks.entries["copy-1"]
	if entry == nil || entry.refs != 2 {
		t.Fatalf("entry = %+v, want two references", entry)
	}
	locks.mu.Unlock()

	first()
	second()
	locks.mu.Lock()
	defer locks.mu.Unlock()
	if len(locks.entries) != 0 {
		t.Fatalf("retained lock entries = %d, want 0", len(locks.entries))
	}
}

func newSerializedRoutingFixture(
	t *testing.T,
	installer Installer,
	applicationVersion string,
	instanceVersion string,
	status InstanceStatus,
) (*Service, *testEventSource, *serializationHost, *observedGetStore, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	application := serializedSubscriberApplication(applicationVersion)
	store := newObservedGetStore(serializedSubscriberInstance(instanceVersion, status))
	host := &serializationHost{notified: make(chan applicationapi.Instance, 4)}
	source := newTestEventSource()
	service := New(
		eventTestRegistry{"consumer": application},
		store,
		installer,
		nil,
		nil,
		WithBackendHost(host),
		WithEventSource(ctx, source),
	)
	return service, source, host, store, cancel
}

func serializedSubscriberApplication(version string) Application {
	application := subscriberApplication("consumer", "remote.applications", "installed")
	application.Version = version
	application.Install = "infra/install.sh"
	application.Service = &ApplicationService{
		Name: "consumer", Command: []string{"/usr/local/bin/consumer"},
	}
	return application
}

func serializedSubscriberInstance(version string, status InstanceStatus) Instance {
	return Instance{
		ID:                 "copy-1",
		ApplicationID:      "consumer",
		ApplicationVersion: version,
		Name:               "Consumer",
		Scope:              ScopeGlobal,
		ContainerName:      "futrx-app-copy-1",
		Status:             status,
	}
}

func publishRoutingTestEvent(source *testEventSource) {
	source.publish(applicationapi.Event{
		Source: applicationapi.EventSource{
			ApplicationID: "remote", Publisher: "remote.applications",
		},
		Name: "installed", Version: 1,
	})
}

func awaitSignal(t *testing.T, signal <-chan struct{}, description string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %s", description)
	}
}

func awaitError(t *testing.T, result <-chan error, description string) error {
	t.Helper()
	select {
	case err := <-result:
		return err
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %s", description)
		return nil
	}
}

func assertNoSerializedNotification(t *testing.T, notifications <-chan applicationapi.Instance) {
	t.Helper()
	select {
	case instance := <-notifications:
		t.Fatalf("unexpected notification to %s", instance.ID)
	default:
	}
}

func assertNotSignaled(t *testing.T, signal <-chan struct{}, description string) {
	t.Helper()
	select {
	case <-signal:
		t.Fatal(description)
	case <-time.After(25 * time.Millisecond):
	}
}

func lifecycleEventKinds(events []recordedApplicationLifecycleEvent) []string {
	kinds := make([]string, 0, len(events))
	for _, event := range events {
		kinds = append(kinds, event.kind)
	}
	return kinds
}
