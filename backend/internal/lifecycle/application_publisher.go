package lifecycle

import "context"

// ApplicationCatalogState identifies a successful catalog mutation.
type ApplicationCatalogState string

const (
	ApplicationAdded   ApplicationCatalogState = "added"
	ApplicationUpdated ApplicationCatalogState = "updated"
	ApplicationDeleted ApplicationCatalogState = "deleted"
)

// ApplicationCatalogEvent identifies a catalog mutation without carrying the
// uploaded archive, manifest configuration, or any installed-instance data.
type ApplicationCatalogEvent struct {
	State         ApplicationCatalogState
	ApplicationID string
}

// ApplicationInstanceState identifies a successful installed-copy transition.
type ApplicationInstanceState string

const (
	ApplicationInstalled   ApplicationInstanceState = "installed"
	ApplicationUninstalled ApplicationInstanceState = "uninstalled"
	ApplicationStarted     ApplicationInstanceState = "started"
	ApplicationStopped     ApplicationInstanceState = "stopped"
)

// ApplicationInstanceEvent identifies one installed copy without exposing its
// environment, credentials, container address, or installer output.
type ApplicationInstanceEvent struct {
	State         ApplicationInstanceState
	ApplicationID string
	InstanceID    string
	Scope         string
	ProjectID     string
}

// ApplicationCatalogSubscriber receives application catalog lifecycle events.
type ApplicationCatalogSubscriber interface {
	OnApplicationCatalog(context.Context, ApplicationCatalogEvent)
}

// ApplicationInstanceSubscriber receives installed-copy lifecycle events.
type ApplicationInstanceSubscriber interface {
	OnApplicationInstance(context.Context, ApplicationInstanceEvent)
}

// ApplicationPublisher owns the two precise application lifecycle streams:
// server-wide catalog mutations and transitions of an installed copy.
type ApplicationPublisher struct {
	catalog   eventDispatcher[ApplicationCatalogEvent]
	instances eventDispatcher[ApplicationInstanceEvent]
}

// NewApplicationPublisher creates an application publisher with no subscribers.
func NewApplicationPublisher() *ApplicationPublisher {
	return &ApplicationPublisher{}
}

// SubscribeCatalog registers a catalog subscriber and returns an idempotent
// function that removes it.
func (p *ApplicationPublisher) SubscribeCatalog(
	subscriber ApplicationCatalogSubscriber,
) (unsubscribe func()) {
	return p.catalog.subscribe(func(ctx context.Context, event ApplicationCatalogEvent) {
		subscriber.OnApplicationCatalog(ctx, event)
	})
}

// SubscribeInstances registers an installed-copy subscriber and returns an
// idempotent function that removes it.
func (p *ApplicationPublisher) SubscribeInstances(
	subscriber ApplicationInstanceSubscriber,
) (unsubscribe func()) {
	return p.instances.subscribe(func(ctx context.Context, event ApplicationInstanceEvent) {
		subscriber.OnApplicationInstance(ctx, event)
	})
}

// PublishApplicationAdded reports that a package entered the live catalog.
func (p *ApplicationPublisher) PublishApplicationAdded(ctx context.Context, applicationID string) {
	p.publishCatalog(ctx, ApplicationAdded, applicationID)
}

// PublishApplicationUpdated reports that an existing package was replaced in
// the live catalog.
func (p *ApplicationPublisher) PublishApplicationUpdated(ctx context.Context, applicationID string) {
	p.publishCatalog(ctx, ApplicationUpdated, applicationID)
}

// PublishApplicationDeleted reports that a package left the live catalog.
func (p *ApplicationPublisher) PublishApplicationDeleted(ctx context.Context, applicationID string) {
	p.publishCatalog(ctx, ApplicationDeleted, applicationID)
}

// PublishApplicationInstalled reports that an installed copy became usable.
func (p *ApplicationPublisher) PublishApplicationInstalled(
	ctx context.Context,
	applicationID, instanceID, scope, projectID string,
) {
	p.publishInstance(ctx, ApplicationInstalled, applicationID, instanceID, scope, projectID)
}

// PublishApplicationUninstalled reports that an installed copy was removed.
func (p *ApplicationPublisher) PublishApplicationUninstalled(
	ctx context.Context,
	applicationID, instanceID, scope, projectID string,
) {
	p.publishInstance(ctx, ApplicationUninstalled, applicationID, instanceID, scope, projectID)
}

// PublishApplicationStarted reports that an existing stopped copy became running.
func (p *ApplicationPublisher) PublishApplicationStarted(
	ctx context.Context,
	applicationID, instanceID, scope, projectID string,
) {
	p.publishInstance(ctx, ApplicationStarted, applicationID, instanceID, scope, projectID)
}

// PublishApplicationStopped reports that an existing running copy became stopped.
func (p *ApplicationPublisher) PublishApplicationStopped(
	ctx context.Context,
	applicationID, instanceID, scope, projectID string,
) {
	p.publishInstance(ctx, ApplicationStopped, applicationID, instanceID, scope, projectID)
}

func (p *ApplicationPublisher) publishCatalog(
	ctx context.Context,
	state ApplicationCatalogState,
	applicationID string,
) {
	p.catalog.publish(ctx, ApplicationCatalogEvent{State: state, ApplicationID: applicationID})
}

func (p *ApplicationPublisher) publishInstance(
	ctx context.Context,
	state ApplicationInstanceState,
	applicationID, instanceID, scope, projectID string,
) {
	p.instances.publish(ctx, ApplicationInstanceEvent{
		State:         state,
		ApplicationID: applicationID,
		InstanceID:    instanceID,
		Scope:         scope,
		ProjectID:     projectID,
	})
}
