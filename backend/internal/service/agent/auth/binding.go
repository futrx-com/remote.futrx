package auth

import (
	"context"
	"errors"
	"sync"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
)

type Flow string

const (
	FlowCode     Flow = "code"
	FlowDevice   Flow = "device"
	FlowAPIKey   Flow = "api-key"
	FlowExternal Flow = "external"
)

var ErrUnsupportedFlow = errors.New("operation is not supported by this agent auth flow")

// LoginSnapshot is the provider-neutral state shared by managed code and
// device flows. URL is the page the user opens; code flows additionally set
// AwaitingCode, while device flows may supply UserCode and ExpiresAt.
type LoginSnapshot struct {
	Active       bool   `json:"active"`
	URL          string `json:"url,omitempty"`
	AwaitingCode bool   `json:"awaitingCode,omitempty"`
	UserCode     string `json:"userCode,omitempty"`
	StartedAt    int64  `json:"startedAt,omitempty"`
	ExpiresAt    int64  `json:"expiresAt,omitempty"`
	Completed    bool   `json:"completed,omitempty"`
	Error        string `json:"error,omitempty"`
}

// Snapshot is the stable auth shape consumed by provider-neutral clients.
// Raw provider status remains available on the legacy routes.
type Snapshot struct {
	Authenticated bool          `json:"authenticated"`
	Warning       string        `json:"warning,omitempty"`
	Login         LoginSnapshot `json:"login"`
}

// Binding is the transport-neutral view of one configured agent auth caller.
// It lets inbound adapters expose HTTP or WebSocket protocols without importing
// a concrete agent package or knowing its CLI policy.
type Binding struct {
	id            agent.ProviderID
	flow          Flow
	status        func() any
	subscribe     func() Subscription
	authenticated func() bool
	snapshot      func() Snapshot
	snapshotSub   func() Subscription
	warning       func() string

	startCode        func(context.Context) (CodeStartResult, error)
	submitCode       func(context.Context, string) error
	cancelCode       func(context.Context) error
	isCodeInputError func(error) bool
	startDevice      func(context.Context) (DeviceState, error)
	setAPIKey        func(context.Context, string) error
	deleteAPIKey     func(context.Context) error
}

func NewCodeBinding(id agent.ProviderID, service *CodeService) Binding {
	binding := Binding{id: id, flow: FlowCode}
	if service == nil {
		return binding
	}
	binding.status = func() any { return service.Status() }
	binding.subscribe = statusSubscription(service.Subscribe)
	binding.authenticated = service.Authenticated
	binding.snapshot = func() Snapshot {
		status := service.Status()
		return Snapshot{
			Authenticated: status.Authenticated,
			Login: LoginSnapshot{
				Active:       status.Login.Active,
				URL:          status.Login.AuthURL,
				AwaitingCode: status.Login.AwaitingCode,
				StartedAt:    status.Login.StartedAt,
				Completed:    status.Login.Completed,
				Error:        status.Login.Error,
			},
		}
	}
	binding.snapshotSub = binding.subscribe
	binding.startCode = service.Start
	binding.submitCode = service.SubmitCode
	binding.cancelCode = service.Cancel
	binding.isCodeInputError = service.IsInputError
	return binding
}

func NewDeviceBinding[S any](id agent.ProviderID, service *DeviceService[S]) Binding {
	binding := Binding{id: id, flow: FlowDevice}
	if service == nil {
		return binding
	}
	binding.status = func() any { return service.Status() }
	binding.subscribe = statusSubscription(service.Subscribe)
	binding.authenticated = service.Authenticated
	binding.snapshot = func() Snapshot {
		state := service.LoginState()
		return Snapshot{
			Authenticated: service.Authenticated(),
			Login: LoginSnapshot{
				Active:    state.Active,
				URL:       state.VerificationURI,
				UserCode:  state.UserCode,
				StartedAt: state.StartedAt,
				ExpiresAt: state.ExpiresAt,
				Completed: state.Completed,
				Error:     state.Error,
			},
		}
	}
	binding.snapshotSub = binding.subscribe
	binding.startDevice = service.StartDeviceLogin
	return binding
}

// NewExternalBinding describes authentication that is completed outside
// Remote's managed code/device flows. It intentionally has no status stream
// or mutation callbacks; callers can use the module descriptor to present the
// provider-owned sign-in instructions instead.
func NewExternalBinding(id agent.ProviderID) Binding {
	return Binding{
		id: id, flow: FlowExternal,
		snapshot: func() Snapshot { return Snapshot{} },
	}
}

// NewAPIKeyBinding exposes a write-only managed credential flow. Status and
// subscriptions reveal only whether a key exists; the key is never returned.
func NewAPIKeyBinding(id agent.ProviderID, service *APIKeyService) Binding {
	binding := Binding{id: id, flow: FlowAPIKey}
	if service == nil {
		return binding
	}
	binding.status = func() any { return service.Status() }
	binding.subscribe = statusSubscription(service.Subscribe)
	binding.authenticated = service.Authenticated
	binding.snapshot = func() Snapshot {
		return Snapshot{Authenticated: service.Authenticated()}
	}
	binding.snapshotSub = binding.subscribe
	binding.setAPIKey = service.Set
	binding.deleteAPIKey = service.Delete
	return binding
}

// WithWarning adds a live provider-specific diagnostic to the normalized
// snapshot without leaking the provider's raw status shape to clients.
func (b Binding) WithWarning(warning func() string) Binding {
	b.warning = warning
	return b
}

func (b Binding) ID() agent.ProviderID { return b.id }

func (b Binding) Flow() Flow { return b.flow }

func (b Binding) Available() bool { return b.status != nil && b.subscribe != nil }

func (b Binding) Authenticated() bool {
	return b.authenticated != nil && b.authenticated()
}

func (b Binding) Status() any {
	if b.status == nil {
		return nil
	}
	return b.status()
}

func (b Binding) Snapshot() Snapshot {
	if b.snapshot == nil {
		return Snapshot{}
	}
	snapshot := b.snapshot()
	if b.warning != nil {
		snapshot.Warning = b.warning()
	}
	return snapshot
}

// Subscribe returns a type-erased view over the caller's original status
// channel. No bridge channel is introduced, so buffering and slow-subscriber
// behavior remain owned by the underlying auth service.
func (b Binding) Subscribe() (Subscription, error) {
	if b.subscribe == nil {
		return Subscription{}, ErrUnsupportedFlow
	}
	return b.subscribe(), nil
}

// SubscribeSnapshots converts provider-specific status notifications into
// the stable auth contract while retaining the original service's buffering
// and subscriber lifetime.
func (b Binding) SubscribeSnapshots() (Subscription, error) {
	if b.snapshotSub == nil {
		return Subscription{}, ErrUnsupportedFlow
	}
	updates := b.snapshotSub()
	return Subscription{
		next: func(ctx context.Context) (any, bool) {
			if _, ok := updates.Next(ctx); !ok {
				return nil, false
			}
			return b.Snapshot(), true
		},
		close: updates.Close,
	}, nil
}

func (b Binding) StartCode(ctx context.Context) (CodeStartResult, error) {
	if b.startCode == nil {
		return CodeStartResult{}, ErrUnsupportedFlow
	}
	return b.startCode(ctx)
}

func (b Binding) SubmitCode(ctx context.Context, code string) error {
	if b.submitCode == nil {
		return ErrUnsupportedFlow
	}
	return b.submitCode(ctx, code)
}

func (b Binding) CancelCode(ctx context.Context) error {
	if b.cancelCode == nil {
		return ErrUnsupportedFlow
	}
	return b.cancelCode(ctx)
}

func (b Binding) IsCodeInputError(err error) bool {
	return b.isCodeInputError != nil && b.isCodeInputError(err)
}

func (b Binding) StartDevice(ctx context.Context) (DeviceState, error) {
	if b.startDevice == nil {
		return DeviceState{}, ErrUnsupportedFlow
	}
	return b.startDevice(ctx)
}

func (b Binding) SetAPIKey(ctx context.Context, key string) error {
	if b.setAPIKey == nil {
		return ErrUnsupportedFlow
	}
	return b.setAPIKey(ctx, key)
}

func (b Binding) DeleteAPIKey(ctx context.Context) error {
	if b.deleteAPIKey == nil {
		return ErrUnsupportedFlow
	}
	return b.deleteAPIKey(ctx)
}

// Subscription reads concrete provider status values from their original
// channel while exposing a transport-neutral value to callers.
type Subscription struct {
	next  func(context.Context) (any, bool)
	close func()
}

func (s Subscription) Next(ctx context.Context) (any, bool) {
	if s.next == nil {
		return nil, false
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return s.next(ctx)
}

func (s Subscription) Close() {
	if s.close != nil {
		s.close()
	}
}

func statusSubscription[S any](subscribe func() (<-chan S, func())) func() Subscription {
	return func() Subscription {
		statuses, unsubscribe := subscribe()
		var closeOnce sync.Once
		return Subscription{
			next: func(ctx context.Context) (any, bool) {
				select {
				case status, ok := <-statuses:
					return status, ok
				case <-ctx.Done():
					return nil, false
				}
			},
			close: func() {
				closeOnce.Do(unsubscribe)
			},
		}
	}
}
