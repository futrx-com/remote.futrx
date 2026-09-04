package agent

import "context"

// ProjectWorkspacePath is the stable mount point for a project's workspace
// inside its execution container. Host-side project paths must never be sent
// to an in-container agent process.
const ProjectWorkspacePath = "/workspace"

// ProjectID is the provider-facing identity of a Remote project. It is kept
// independent from the project service's storage and transport models so agent
// modules depend only on this narrow execution port.
type ProjectID string

type ProjectStatus string

const ProjectStatusRunning ProjectStatus = "running"

// Project contains only the project state an agent needs to prepare and run a
// CLI inside its workspace container.
type Project struct {
	ID            ProjectID
	ContainerName string
	Status        ProjectStatus
}

// ProjectSecret is an environment variable made available to an agent run.
type ProjectSecret struct {
	Key   string
	Value string
}

// ProjectResolver is the complete project surface available to shared agent
// execution services. Service-layer project models are translated at the
// composition boundary and never leak into provider packages.
type ProjectResolver interface {
	Get(context.Context, ProjectID) (Project, error)
	Start(context.Context, ProjectID) (Project, error)
	ListSecrets(context.Context, ProjectID) ([]ProjectSecret, error)
}

// ProjectPreparationRequest contains only the provider-neutral run state used
// to reconcile and prepare a project workspace.
type ProjectPreparationRequest struct {
	ProjectID           ProjectID
	ConversationID      string
	EnableBrowser       bool
	EnableScheduleTools bool
}

// PreparedProject is the stable container target and environment policy
// returned to a provider after shared workspace preparation succeeds.
type PreparedProject struct {
	ID            ProjectID
	ContainerName string
	Secrets       []ProjectSecret
}

// ProjectPreparer owns the shared project lifecycle and provisioning workflow.
// Providers retain responsibility for their CLI arguments and wire protocol.
type ProjectPreparer interface {
	Prepare(context.Context, ProjectPreparationRequest, func(Event)) (PreparedProject, error)
}
