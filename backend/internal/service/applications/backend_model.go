package applications

import "github.com/futrx-com/remote.futrx.com/pkg/appplugin"

// BackendAccess is the audience the transport lets reach an application's plugin.
// It is the one capability control the platform enforces on a plugin's behalf;
// everything finer-grained is the plugin's own job, using Request.Caller.
type BackendAccess string

const (
	// BackendAccessRegistered lets any signed-in user call the plugin, which
	// is what a plugin backing a UI extension needs, since its extension
	// renders for every user the application is installed for.
	BackendAccessRegistered BackendAccess = "registered"
	// BackendAccessAdmin restricts calls to server administrators.
	BackendAccessAdmin BackendAccess = "admin"
)

// Valid reports whether a is a known access level.
func (a BackendAccess) Valid() bool {
	return a == BackendAccessRegistered || a == BackendAccessAdmin
}

// ApplicationBackend describes the Go plugin an application ships in its backend/
// directory. Like ApplicationUI, the directory is what opts the application in; this block
// only overrides the defaults. It is nil when the application ships no plugin.
type ApplicationBackend struct {
	// Access is who may call the plugin. Empty means BackendAccessRegistered.
	Access BackendAccess `json:"access,omitempty"`
	// TimeoutMS bounds a single call. Empty means DefaultBackendTimeoutMS.
	TimeoutMS int `json:"timeoutMs,omitempty"`
}

// DefaultBackendTimeoutMS bounds one plugin call when the application does not say.
// A plugin is a child process the request goroutine waits on, so an unbounded
// call would be an unbounded held connection.
const DefaultBackendTimeoutMS = 15000

// Timeout returns the effective per-call timeout in milliseconds.
func (b ApplicationBackend) Timeout() int {
	if b.TimeoutMS <= 0 {
		return DefaultBackendTimeoutMS
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

// BackendInstance identifies one running plugin an extension may address. An
// application installed both globally and in a project has one process for each, so
// the SPA needs the instance, not just the application, to call the right one.
type BackendInstance struct {
	InstanceID string `json:"instanceId"`
	Scope      Scope  `json:"scope"`
	ProjectID  string `json:"projectId,omitempty"`
}

// BackendDescriptor is what an instance's plugin reports about itself, plus
// the identity of the instance serving it.
type BackendDescriptor struct {
	InstanceID    string               `json:"instanceId"`
	ApplicationID string               `json:"applicationId"`
	Descriptor    appplugin.Descriptor `json:"descriptor"`
	Access        BackendAccess        `json:"access"`
	TimeoutMS     int                  `json:"timeoutMs"`
}
