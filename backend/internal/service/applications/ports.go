package applications

import "context"

// Registry provides the installable catalog loaded from embedded image
// definitions.
type Registry interface {
	List() []Image
	Get(id string) (Image, bool)
}

// InstallSpec is everything Installer needs to realize an instance in a
// container. It is derived from an Image plus the user's resolved inputs.
type InstallSpec struct {
	Image    Image
	Instance Instance
}

// Installer realizes and controls app instances inside containers. It owns the
// lxc-facing side: running the install script, systemd start/stop, and the
// host proxy device that exposes the port.
type Installer interface {
	// Install (re)runs the image's install script and (re)creates the proxy
	// device so the app is reachable on the host.
	Install(ctx context.Context, spec InstallSpec) error
	// Start starts the app's service and ensures its proxy device exists.
	Start(ctx context.Context, spec InstallSpec) error
	// Stop stops the app's service and removes its proxy device so the host
	// port is released.
	Stop(ctx context.Context, spec InstallSpec) error
	// Uninstall removes the proxy device and, for global scope, deletes the
	// dedicated container.
	Uninstall(ctx context.Context, spec InstallSpec) error
	// Expose (re)creates only the host proxy device for an instance, without
	// re-running the install script. Used for cheap external-port changes.
	Expose(ctx context.Context, spec InstallSpec) error
}

// Store persists installed instances. Global instances are keyed only by ID;
// project instances are additionally partitioned by project.
type Store interface {
	ListGlobal(ctx context.Context) ([]Instance, error)
	ListProject(ctx context.Context, projectID string) ([]Instance, error)
	// ListAll returns every instance across all scopes, used for host-port
	// conflict checks.
	ListAll(ctx context.Context) ([]Instance, error)
	Get(ctx context.Context, id string) (Instance, bool, error)
	Put(ctx context.Context, inst Instance) error
	Delete(ctx context.Context, id string) error
}

// ProjectContainers resolves and readies a project's container. Implemented by
// the project service so applications never depends on it directly.
type ProjectContainers interface {
	// ContainerName returns the LXD container name (slug) for a project.
	ContainerName(ctx context.Context, projectID string) (string, error)
	// EnsureRunning converges the project container to a running state.
	EnsureRunning(ctx context.Context, projectID string) error
}

// PortAllocator picks a free host port for a new proxy device.
type PortAllocator interface {
	// Allocate returns the first free host port at or after preferred, skipping
	// any port in taken (ports the caller already reserved this pass).
	Allocate(ctx context.Context, bindAddress string, preferred int, taken map[int]bool) (int, error)
}
