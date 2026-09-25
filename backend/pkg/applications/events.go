package applications

import "encoding/json"

// MaxEventPayloadBytes is the maximum encoded JSON payload accepted for one
// publication. The RPC client checks this before transport and the host checks
// it again at the trust boundary.
const MaxEventPayloadBytes = 64 << 10

const (
	// RemoteApplicationsPublisher is Remote's canonical publisher for catalog
	// and installed-application lifecycle events.
	RemoteApplicationsPublisher = "remote.applications"
	// RemoteApplicationsVersion is the current payload contract version for
	// every event emitted by RemoteApplicationsPublisher.
	RemoteApplicationsVersion = 1

	RemoteApplicationAdded       = "added"
	RemoteApplicationUpdated     = "updated"
	RemoteApplicationDeleted     = "deleted"
	RemoteApplicationInstalled   = "installed"
	RemoteApplicationUninstalled = "uninstalled"
	RemoteApplicationStarted     = "started"
	RemoteApplicationStopped     = "stopped"
)

// IsRemoteApplicationEvent reports whether name belongs to Remote's canonical
// application lifecycle publisher.
func IsRemoteApplicationEvent(name string) bool {
	switch name {
	case RemoteApplicationAdded,
		RemoteApplicationUpdated,
		RemoteApplicationDeleted,
		RemoteApplicationInstalled,
		RemoteApplicationUninstalled,
		RemoteApplicationStarted,
		RemoteApplicationStopped:
		return true
	default:
		return false
	}
}

// PublisherDeclaration describes one publisher an application exposes. Name
// is local to the application; Remote qualifies it with the application ID
// when routing events so independently developed applications cannot collide.
type PublisherDeclaration struct {
	Name   string             `json:"name"`
	Events []EventDeclaration `json:"events"`
}

// EventDeclaration describes one version of an event a publisher may emit.
// Version starts at one and moves only when the payload contract changes.
type EventDeclaration struct {
	Name        string `json:"name"`
	Version     int    `json:"version"`
	Description string `json:"description,omitempty"`
}

// Subscription selects events from one canonical publisher. Publisher is the
// qualified name exposed by Remote (for example "remote.applications" or
// "applications.hello-remote.greetings"); Events contains the event names to
// deliver.
type Subscription struct {
	Publisher string   `json:"publisher"`
	Events    []string `json:"events"`
}

// EventSource is the identity Remote stamps onto a publication before it is
// delivered. Application backends never supply this value themselves.
type EventSource struct {
	ApplicationID string `json:"applicationId"`
	InstanceID    string `json:"instanceId,omitempty"`
	Scope         string `json:"scope,omitempty"`
	ProjectID     string `json:"projectId,omitempty"`
	Publisher     string `json:"publisher"`
}

// Publication is an application's request to publish one declared event.
// Publisher is the manifest-local publisher name. Remote validates the
// declaration and stamps the installed instance identity onto the Event that
// subscribers receive.
type Publication struct {
	Publisher string          `json:"publisher"`
	Event     string          `json:"event"`
	Version   int             `json:"version"`
	Payload   json.RawMessage `json:"payload,omitempty"`
}

// Event is one validated publication delivered to a subscriber. Source is
// trusted host metadata; Payload remains application-defined JSON governed by
// the matching manifest event declaration.
type Event struct {
	Source  EventSource     `json:"source"`
	Name    string          `json:"name"`
	Version int             `json:"version"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// EventEmitter is the core-owned runtime capability an application uses to
// submit one manifest-declared event. Remote validates the publication and
// stamps its trusted source identity before dispatch. A nil error means the
// event was accepted by core; it does not mean an asynchronous subscriber
// completed or that a best-effort overload queue could not later drop it.
type EventEmitter interface {
	Emit(Publication) error
}

// EventSubscriber is an optional capability implemented by a backend that
// consumes events declared in its manifest. Calls may overlap with Handle and
// with other OnEvent calls, so implementations must be concurrency-safe.
type EventSubscriber interface {
	OnEvent(Event) error
}
