package lifecycle

import (
	"context"
	"encoding/json"

	"github.com/futrx-com/remote.futrx.com/pkg/applications"
)

const (
	// CoreApplicationEventPublisher is Remote's reserved publisher for catalog
	// and installed-application lifecycle facts.
	CoreApplicationEventPublisher = applications.RemoteApplicationsPublisher
	// CoreApplicationEventVersion is the schema version shared by the
	// declaration, event envelope, and JSON payload.
	CoreApplicationEventVersion = applications.RemoteApplicationsVersion
	coreApplicationSource       = "remote"
)

// ApplicationEventSink is the dynamic event capability used by the bridge.
// EventBus implements it; keeping the port narrow makes bridge tests and future
// transports independent of the concrete bus.
type ApplicationEventSink interface {
	Publish(context.Context, applications.Event)
}

// ApplicationEventBridge translates core's typed application lifecycle into
// the public dynamic event contract applications can subscribe to.
type ApplicationEventBridge struct {
	events ApplicationEventSink
}

var (
	_ ApplicationCatalogSubscriber  = (*ApplicationEventBridge)(nil)
	_ ApplicationInstanceSubscriber = (*ApplicationEventBridge)(nil)
)

// NewApplicationEventBridge creates a lifecycle subscriber that forwards to
// the supplied dynamic event sink.
func NewApplicationEventBridge(events ApplicationEventSink) *ApplicationEventBridge {
	return &ApplicationEventBridge{events: events}
}

// OnApplicationCatalog publishes a server-wide catalog event. Catalog events
// have no instance routing fields because they do not belong to an installed
// copy or project.
func (b *ApplicationEventBridge) OnApplicationCatalog(
	ctx context.Context,
	event ApplicationCatalogEvent,
) {
	name, ok := catalogEventName(event.State)
	if !ok {
		return
	}
	b.events.Publish(ctx, applications.Event{
		Source: applications.EventSource{
			ApplicationID: coreApplicationSource,
			Publisher:     CoreApplicationEventPublisher,
		},
		Name:    name,
		Version: CoreApplicationEventVersion,
		Payload: marshalCoreApplicationPayload(coreApplicationPayload{
			Version:       CoreApplicationEventVersion,
			ApplicationID: event.ApplicationID,
		}),
	})
}

// OnApplicationInstance publishes an installed-copy transition. The subject's
// instance, scope, and project are stamped into Source so subscribers can
// route without decoding payload; the subject application remains explicit in
// the payload because Source.ApplicationID identifies Remote as the emitter.
func (b *ApplicationEventBridge) OnApplicationInstance(
	ctx context.Context,
	event ApplicationInstanceEvent,
) {
	name, ok := instanceEventName(event.State)
	if !ok {
		return
	}
	b.events.Publish(ctx, applications.Event{
		Source: applications.EventSource{
			ApplicationID: coreApplicationSource,
			InstanceID:    event.InstanceID,
			Scope:         event.Scope,
			ProjectID:     event.ProjectID,
			Publisher:     CoreApplicationEventPublisher,
		},
		Name:    name,
		Version: CoreApplicationEventVersion,
		Payload: marshalCoreApplicationPayload(coreApplicationPayload{
			Version:       CoreApplicationEventVersion,
			ApplicationID: event.ApplicationID,
			InstanceID:    event.InstanceID,
			Scope:         event.Scope,
			ProjectID:     event.ProjectID,
		}),
	})
}

type coreApplicationPayload struct {
	Version       int    `json:"version"`
	ApplicationID string `json:"applicationId"`
	InstanceID    string `json:"instanceId,omitempty"`
	Scope         string `json:"scope,omitempty"`
	ProjectID     string `json:"projectId,omitempty"`
}

func marshalCoreApplicationPayload(payload coreApplicationPayload) json.RawMessage {
	// This struct contains only strings and an integer, so encoding/json cannot
	// fail. Keeping marshaling here gives every canonical event one wire shape.
	encoded, _ := json.Marshal(payload)
	return encoded
}

func catalogEventName(state ApplicationCatalogState) (string, bool) {
	switch state {
	case ApplicationAdded:
		return applications.RemoteApplicationAdded, true
	case ApplicationUpdated:
		return applications.RemoteApplicationUpdated, true
	case ApplicationDeleted:
		return applications.RemoteApplicationDeleted, true
	default:
		return "", false
	}
}

func instanceEventName(state ApplicationInstanceState) (string, bool) {
	switch state {
	case ApplicationInstalled:
		return applications.RemoteApplicationInstalled, true
	case ApplicationUninstalled:
		return applications.RemoteApplicationUninstalled, true
	case ApplicationStarted:
		return applications.RemoteApplicationStarted, true
	case ApplicationStopped:
		return applications.RemoteApplicationStopped, true
	default:
		return "", false
	}
}
