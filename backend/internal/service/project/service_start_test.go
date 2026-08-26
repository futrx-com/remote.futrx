package project

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestStartPersistsAndReturnsLaunchError(t *testing.T) {
	wantErr := errors.New("prepare workspace: dangling browser lock")
	repo := &startTestRepository{meta: Meta{
		ID:            ID("abcd"),
		Name:          "project",
		ContainerName: "project",
		Status:        StatusMissing,
	}}
	lifecycle := &startTestLifecycle{state: ContainerStateMissing, launchErr: wantErr}
	service := New(repo, ContainerDependencies{Lifecycle: lifecycle}, nil, nil)

	got, err := service.Start(context.Background(), repo.meta.ID)
	if !errors.Is(err, wantErr) {
		t.Fatalf("Start() error = %v, want %v", err, wantErr)
	}
	if got.Status != StatusError || got.ErrorMsg != wantErr.Error() {
		t.Fatalf("Start() meta = %#v, want persisted launch error", got)
	}
	if repo.meta.Status != StatusError || repo.meta.ErrorMsg != wantErr.Error() {
		t.Fatalf("repository meta = %#v, want persisted launch error", repo.meta)
	}
}

func TestConcurrentStartSerializesContainerEnsure(t *testing.T) {
	repo := &startTestRepository{meta: Meta{
		ID:            ID("abcd"),
		Name:          "project",
		ContainerName: "project",
		Status:        StatusRunning,
	}}
	lifecycle := &startTestLifecycle{state: ContainerStateMissing}
	service := New(repo, ContainerDependencies{Lifecycle: lifecycle}, nil, nil)

	start := make(chan struct{})
	errs := make(chan error, 2)
	for range 2 {
		go func() {
			<-start
			_, err := service.Start(context.Background(), repo.meta.ID)
			errs <- err
		}()
	}
	close(start)
	for range 2 {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}

	lifecycle.mu.Lock()
	defer lifecycle.mu.Unlock()
	if lifecycle.launchCalls != 2 {
		t.Fatalf("Ensure() calls = %d, want 2 serialized idempotent checks", lifecycle.launchCalls)
	}
}

func TestStartDelegatesFrozenRecoveryToContainerEnsure(t *testing.T) {
	repo := &startTestRepository{meta: Meta{
		ID:            ID("abcd"),
		Name:          "project",
		ContainerName: "project",
		Status:        StatusRunning,
	}}
	lifecycle := &startTestLifecycle{state: ContainerStateFrozen}
	service := New(repo, ContainerDependencies{Lifecycle: lifecycle}, nil, nil)

	got, err := service.Start(context.Background(), repo.meta.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != StatusRunning {
		t.Fatalf("Start() status = %q, want %q", got.Status, StatusRunning)
	}

	lifecycle.mu.Lock()
	defer lifecycle.mu.Unlock()
	if lifecycle.launchCalls != 1 {
		t.Fatalf("Ensure() calls = %d, want 1", lifecycle.launchCalls)
	}
	if lifecycle.restartCalls != 0 || lifecycle.startCalls != 0 {
		t.Fatalf("project service bypassed Ensure: restart=%d start=%d", lifecycle.restartCalls, lifecycle.startCalls)
	}
	if lifecycle.state != ContainerStateRunning {
		t.Fatalf("container state = %q, want %q", lifecycle.state, ContainerStateRunning)
	}
}

func TestUpgradeSkipsBusyProjectUnlessExplicitlyIncluded(t *testing.T) {
	repo := &startTestRepository{meta: Meta{
		ID: ID("abcd"), Name: "project", ContainerName: "project", Status: StatusRunning,
	}}
	lifecycle := &startTestLifecycle{state: ContainerStateRunning, busy: true}
	browser := &upgradeTestBrowser{}
	service := New(repo, ContainerDependencies{Lifecycle: lifecycle, Browser: browser}, nil, nil)

	if _, err := service.Upgrade(context.Background(), repo.meta.ID, false); !errors.Is(err, ErrProjectBusy) {
		t.Fatalf("Upgrade() error = %v, want busy", err)
	}
	if lifecycle.launchCalls != 0 {
		t.Fatalf("busy project was changed: ensure calls = %d", lifecycle.launchCalls)
	}
	if browser.stopCalls != 0 {
		t.Fatalf("busy project browser was stopped: calls = %d", browser.stopCalls)
	}

	if _, err := service.Upgrade(context.Background(), repo.meta.ID, true); err != nil {
		t.Fatal(err)
	}
	if lifecycle.launchCalls != 2 {
		t.Fatalf("Ensure() calls = %d, want legacy migration and fresh-container validation", lifecycle.launchCalls)
	}
	if lifecycle.state != ContainerStateRunning {
		t.Fatalf("state = %q, want running", lifecycle.state)
	}
	if browser.stopCalls != 1 {
		t.Fatalf("browser Stop() calls = %d, want 1 before replacement", browser.stopCalls)
	}
}

type upgradeTestBrowser struct {
	stopCalls int
}

func (*upgradeTestBrowser) Ensure(context.Context, string) error { return nil }
func (b *upgradeTestBrowser) Stop(context.Context, string) error {
	b.stopCalls++
	return nil
}
func (*upgradeTestBrowser) StopView(context.Context, string) error { return nil }
func (*upgradeTestBrowser) Status(context.Context, string) (AgentBrowserInfo, error) {
	return AgentBrowserInfo{}, nil
}
func (*upgradeTestBrowser) Port() int { return 6080 }

func TestRunStateTransitionsWaitForConcurrentStart(t *testing.T) {
	tests := []struct {
		name      string
		wantCall  string
		operation func(context.Context, *Service, ID) error
	}{
		{
			name:     "stop",
			wantCall: "stop",
			operation: func(ctx context.Context, service *Service, id ID) error {
				_, err := service.Stop(ctx, id)
				return err
			},
		},
		{
			name:     "restart",
			wantCall: "restart",
			operation: func(ctx context.Context, service *Service, id ID) error {
				_, err := service.Restart(ctx, id)
				return err
			},
		},
		{
			name:     "delete",
			wantCall: "delete",
			operation: func(ctx context.Context, service *Service, id ID) error {
				return service.Delete(ctx, id)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := &startTestRepository{meta: Meta{
				ID:            ID("abcd"),
				Name:          "project",
				ContainerName: "project",
				Status:        StatusRunning,
			}}
			launchStarted := make(chan struct{})
			releaseLaunch := make(chan struct{})
			transitionCalls := make(chan string, 1)
			lifecycle := &startTestLifecycle{
				state:           ContainerStateMissing,
				launchStarted:   launchStarted,
				releaseLaunch:   releaseLaunch,
				transitionCalls: transitionCalls,
			}
			service := New(repo, ContainerDependencies{Lifecycle: lifecycle}, nil, nil)

			startResult := make(chan error, 1)
			go func() {
				_, err := service.Start(context.Background(), repo.meta.ID)
				startResult <- err
			}()
			<-launchStarted

			operationStarted := make(chan struct{})
			operationResult := make(chan error, 1)
			go func() {
				close(operationStarted)
				operationResult <- test.operation(context.Background(), service, repo.meta.ID)
			}()
			<-operationStarted
			select {
			case call := <-transitionCalls:
				close(releaseLaunch)
				<-startResult
				<-operationResult
				t.Fatalf("%s reached lifecycle before concurrent launch completed", call)
			case <-time.After(50 * time.Millisecond):
			}

			close(releaseLaunch)
			if err := <-startResult; err != nil {
				t.Fatalf("Start() error: %v", err)
			}
			if err := <-operationResult; err != nil {
				t.Fatalf("%s error: %v", test.name, err)
			}
			if call := <-transitionCalls; call != test.wantCall {
				t.Fatalf("lifecycle call = %q, want %q", call, test.wantCall)
			}
		})
	}
}

type startTestRepository struct {
	mu   sync.Mutex
	meta Meta
}

func (r *startTestRepository) List(context.Context) ([]Meta, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return []Meta{r.meta}, nil
}

func (r *startTestRepository) Create(_ context.Context, meta Meta) (Meta, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.meta = meta
	return meta, nil
}

func (r *startTestRepository) Get(context.Context, ID) (Meta, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.meta, nil
}

func (r *startTestRepository) GetBySlug(context.Context, string) (Meta, error) {
	return r.Get(context.Background(), r.meta.ID)
}

func (r *startTestRepository) Update(_ context.Context, _ ID, fn func(*Meta)) (Meta, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	fn(&r.meta)
	return r.meta, nil
}

func (r *startTestRepository) SetStatus(_ context.Context, _ ID, status Status, errMsg string) (Meta, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.meta.Status = status
	r.meta.ErrorMsg = errMsg
	return r.meta, nil
}

func (r *startTestRepository) Delete(context.Context, ID) error { return nil }

type startTestLifecycle struct {
	mu              sync.Mutex
	state           ContainerState
	launchErr       error
	launchCalls     int
	lastEnsured     Meta
	startCalls      int
	restartCalls    int
	launchStarted   chan struct{}
	releaseLaunch   <-chan struct{}
	transitionCalls chan<- string
	busy            bool
}

func (l *startTestLifecycle) Available() bool { return true }

func (l *startTestLifecycle) State(context.Context, string) (ContainerState, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.state, nil
}

func (l *startTestLifecycle) Ensure(_ context.Context, project Meta) error {
	l.mu.Lock()
	l.launchCalls++
	l.lastEnsured = project
	launchErr := l.launchErr
	launchStarted := l.launchStarted
	releaseLaunch := l.releaseLaunch
	l.mu.Unlock()

	if launchStarted != nil {
		close(launchStarted)
	}
	if releaseLaunch != nil {
		<-releaseLaunch
	}
	if launchErr != nil {
		return launchErr
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.state = ContainerStateRunning
	return nil
}

func (l *startTestLifecycle) Busy(context.Context, string) (bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.busy, nil
}

func (l *startTestLifecycle) Start(context.Context, string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.startCalls++
	l.state = ContainerStateRunning
	return nil
}

func (l *startTestLifecycle) Stop(context.Context, string) error {
	l.recordTransition("stop", ContainerStateStopped)
	l.mu.Lock()
	l.busy = false
	l.mu.Unlock()
	return nil
}
func (l *startTestLifecycle) Restart(context.Context, string) error {
	l.recordTransition("restart", ContainerStateRunning)
	return nil
}
func (l *startTestLifecycle) Delete(context.Context, string) error {
	l.recordTransition("delete", ContainerStateMissing)
	return nil
}

func (l *startTestLifecycle) recordTransition(name string, state ContainerState) {
	l.mu.Lock()
	if name == "restart" {
		l.restartCalls++
	}
	l.state = state
	transitionCalls := l.transitionCalls
	l.mu.Unlock()
	if transitionCalls != nil {
		transitionCalls <- name
	}
}

func (l *startTestLifecycle) EnsureResources(context.Context, string) error { return nil }
func (l *startTestLifecycle) SetResourceLimits(context.Context, string, ContainerLimits) error {
	return nil
}
