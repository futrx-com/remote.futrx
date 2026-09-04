package project

import "context"

type Repository interface {
	List(ctx context.Context) ([]Meta, error)
	Create(ctx context.Context, meta Meta) (Meta, error)
	Get(ctx context.Context, id ID) (Meta, error)
	GetBySlug(ctx context.Context, slug string) (Meta, error)
	Update(ctx context.Context, id ID, fn func(*Meta)) (Meta, error)
	SetStatus(ctx context.Context, id ID, status Status, errMsg string) (Meta, error)
	Delete(ctx context.Context, id ID) error
}

// ContainerLifecycle is the project service's container state-transition port.
// Implementations may use LXD or any other runtime; project policy only relies
// on these lifecycle operations.
type ContainerLifecycle interface {
	Available() bool
	// Ensure converges a project to one complete, running container with all
	// durable mounts attached.
	Ensure(ctx context.Context, p Meta) error
	Busy(ctx context.Context, containerName string) (bool, error)
	Start(ctx context.Context, containerName string) error
	Stop(ctx context.Context, containerName string) error
	// Restart force-restarts the container from the host, killing any
	// wedged processes inside without needing their cooperation.
	Restart(ctx context.Context, containerName string) error
	Delete(ctx context.Context, containerName string) error
	State(ctx context.Context, containerName string) (ContainerState, error)
	// EnsureResources converges the fleet-default resource envelope (the
	// managed LXD profile) onto an existing container.
	EnsureResources(ctx context.Context, containerName string) error
	// SetResourceLimits applies container-local overrides. Empty values remove
	// overrides so the managed fleet profile becomes effective again.
	SetResourceLimits(ctx context.Context, containerName string, limits ContainerLimits) error
}

// ContainerPolicy supplies the fleet-wide resource policy that per-project
// operations are measured against: the defaults a project inherits, the
// ceiling an override may not pass, live host capacity, and whether the
// storage pool can enforce a disk quota at all.
type ContainerPolicy interface {
	Policy(ctx context.Context) ContainerPolicySnapshot
	// Validate rejects an override above the operator ceiling or above what
	// the host can physically back.
	Validate(ctx context.Context, limits ContainerLimits) error
}

// ContainerAdmission is the aggregate guard consulted before a container is
// launched or started. It exists so a host reboot that autostarts every
// workspace cannot commit more memory than the host has.
type ContainerAdmission interface {
	// AuthorizeStart reports whether one more running container fits.
	// memoryLimit is the candidate's effective ceiling; an empty value means
	// "the fleet default". force is the admin escape hatch.
	AuthorizeStart(ctx context.Context, containerName, memoryLimit string, force bool) error
}

// ContainerEnvironment applies project secrets to future container sessions.
type ContainerEnvironment interface {
	ApplyDiff(ctx context.Context, containerName string, set map[string]string, unset []string) error
}

// ContainerInspector returns a best-effort diagnostic snapshot.
type ContainerInspector interface {
	Inspect(ctx context.Context, containerName string) (ContainerInspect, error)
}

// ContainerNetwork repairs guest network configuration.
type ContainerNetwork interface {
	Repair(ctx context.Context, containerName string) error
}

// ContainerListeners discovers externally reachable guest applications.
type ContainerListeners interface {
	List(ctx context.Context, containerName string) ([]ContainerApp, error)
}

// ContainerBrowser manages the browser capabilities consumed by projects.
type ContainerBrowser interface {
	Ensure(ctx context.Context, containerName string) error
	Stop(ctx context.Context, containerName string) error
	StopView(ctx context.Context, containerName string) error
	Status(ctx context.Context, containerName string) (AgentBrowserInfo, error)
	Port() int
}

// ContainerDependencies groups the independently replaceable container
// capabilities used by Service. A nil capability preserves the behavior of a
// nil container manager for the operations that consume it.
type ContainerDependencies struct {
	Lifecycle   ContainerLifecycle
	Environment ContainerEnvironment
	Inspector   ContainerInspector
	Network     ContainerNetwork
	Listeners   ContainerListeners
	Browser     ContainerBrowser
	Policy      ContainerPolicy
	Admission   ContainerAdmission
}
