package applications

import "github.com/futrx-com/remote.futrx.com/pkg/applications"

// BackendAccess is the audience the transport lets reach an application's backend.
// It is the one capability control the platform enforces on a backend's behalf;
// everything finer-grained is the backend's own job, using Request.Caller.
type BackendAccess string

const (
	// BackendAccessRegistered lets any signed-in user call the backend, which
	// is what a backend backing a UI extension needs, since its extension
	// renders for every user the application is installed for.
	BackendAccessRegistered BackendAccess = "registered"
	// BackendAccessAdmin restricts calls to server administrators.
	BackendAccessAdmin BackendAccess = "admin"
)

// Valid reports whether a is a known access level.
func (a BackendAccess) Valid() bool {
	return a == BackendAccessRegistered || a == BackendAccessAdmin
}

// ApplicationBackend describes the Go backend an application ships in its backend/
// directory. Like ApplicationUI, the directory is what opts the application in; this block
// only overrides the defaults. It is nil when the application ships no backend.
type ApplicationBackend struct {
	// Access is who may call the backend. Empty means BackendAccessRegistered.
	Access BackendAccess `json:"access,omitempty"`
	// TimeoutMS requests the bound for a single call. Empty means
	// DefaultBackendTimeoutMS; values above MaxBackendTimeoutMS remain accepted
	// for compatibility but are capped when the timeout is applied.
	TimeoutMS int `json:"timeoutMs,omitempty"`
}

const (
	// DefaultBackendTimeoutMS bounds one backend call when the application does
	// not say. A backend is a child process the request goroutine waits on, so
	// an unbounded call would be an unbounded held connection.
	DefaultBackendTimeoutMS = 15000
	// MaxBackendTimeoutMS is the largest effective per-call timeout. Manifests
	// may contain a larger legacy value, but runtime operations cap it here so a
	// typo or hostile uploaded package cannot retain request and process
	// resources indefinitely.
	MaxBackendTimeoutMS = 300000
	// MaxEventDeliveryTimeoutMS limits one subscriber's share of the single
	// ordered event-delivery worker. A manifest may allow longer browser calls,
	// but it may not stall all later event recipients for that long.
	MaxEventDeliveryTimeoutMS = 30000
)

// Timeout returns the effective per-call timeout in milliseconds.
func (b ApplicationBackend) Timeout() int {
	if b.TimeoutMS <= 0 {
		return DefaultBackendTimeoutMS
	}
	if b.TimeoutMS > MaxBackendTimeoutMS {
		return MaxBackendTimeoutMS
	}
	return b.TimeoutMS
}

// Audience returns the effective access level.
func (b ApplicationBackend) Audience() BackendAccess {
	if b.Access == "" {
		return BackendAccessRegistered
	}
	return b.Access
}

// BackendInstance identifies one running backend an extension may address. An
// application installed both globally and in a project has one process for each, so
// the SPA needs the instance, not just the application, to call the right one.
type BackendInstance struct {
	InstanceID string `json:"instanceId"`
	Scope      Scope  `json:"scope"`
	ProjectID  string `json:"projectId,omitempty"`
}

// BackendDescriptor is what an instance's backend reports about itself, plus
// the identity of the instance serving it.
type BackendDescriptor struct {
	InstanceID    string                  `json:"instanceId"`
	ApplicationID string                  `json:"applicationId"`
	Descriptor    applications.Descriptor `json:"descriptor"`
	Access        BackendAccess           `json:"access"`
	TimeoutMS     int                     `json:"timeoutMs"`
}
