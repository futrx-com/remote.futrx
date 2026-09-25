package applications

import (
	"context"

	"github.com/futrx-com/remote.futrx.com/pkg/applications"
)

// Registry provides the installable catalog loaded from embedded application
// definitions.
type Registry interface {
	List() []Application
	Get(id string) (Application, bool)
	// UIAsset returns one file from an application's ui/ directory. assetPath is
	// relative to that directory ("scripts/main.js"); anything escaping it, or
	// belonging to an application without a ui/, reports not found.
	UIAsset(applicationID, assetPath string) ([]byte, bool)
}

// InstallSpec is everything Installer needs to realize an instance in a
// container. It is derived from an Application plus the user's resolved inputs.
type InstallSpec struct {
	Application Application
	Instance    Instance
}

// Installer realizes and controls app instances inside containers. It owns the
// lxc-facing side: running the install script, systemd start/stop, and the
// host proxy device that exposes the port.
type Installer interface {
	// Install (re)runs the application's install script and (re)creates the proxy
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

// BackendHost compiles an application's backend/ source and runs it as a child
// process, one per instance, forwarding calls to it. It owns everything
// go-plugin-facing, so the service layer never launches a process itself.
//
// Every method is safe on an instance whose application ships no backend: the service
// checks that before calling, but a host that is asked anyway must not create
// one.
type BackendHost interface {
	// Ensure builds the backend if no current binary is cached, starts a
	// process for the instance, and returns the manifest-enriched descriptor.
	// It is idempotent: a call against an already-running instance returns the
	// descriptor established at connect time.
	Ensure(ctx context.Context, instance applications.Instance) (applications.Descriptor, error)
	// Call forwards one request, starting the backend first if it is not
	// running — which is what makes installed backends survive a server
	// restart without a start sweep.
	Call(ctx context.Context, instance applications.Instance, request applications.Request) (applications.Response, error)
	// Notify delivers one subscribed event, lazily starting the backend after a
	// Remote process restart in the same way Call does.
	Notify(ctx context.Context, instance applications.Instance, event applications.Event) error
	// Stop terminates the instance's backend process, keeping its data
	// directory so a later start resumes with it.
	Stop(ctx context.Context, instanceID string) error
	// Remove stops the backend and deletes the instance's data directory.
	Remove(ctx context.Context, instanceID string) error
	// InvalidateApplication stops every process created from an application and
	// prevents an in-flight launch from surviving a package replacement. The
	// next call rebuilds against the registry's current source and manifest.
	InvalidateApplication(applicationID string)
}

// EventSource is the process-wide stream of validated application and core
// events. The service subscribes once and owns routing those events to running
// application backend instances.
type EventSource interface {
	Subscribe(func(context.Context, applications.Event)) (unsubscribe func())
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

// ApplicationLifecyclePublisher is the success-notification capability used
// by this service. Its concrete publisher and subscriber registry live in
// internal/lifecycle; the producer depends only on the facts it emits.
type ApplicationLifecyclePublisher interface {
	PublishApplicationAdded(context.Context, string)
	PublishApplicationUpdated(context.Context, string)
	PublishApplicationDeleted(context.Context, string)
	PublishApplicationInstalled(context.Context, string, string, string, string)
	PublishApplicationUninstalled(context.Context, string, string, string, string)
	PublishApplicationStarted(context.Context, string, string, string, string)
	PublishApplicationStopped(context.Context, string, string, string, string)
}

// PackageUpload is one archive submitted for installation into the catalog.
type PackageUpload struct {
	// Filename is the client's name for the archive. It is recorded, never
	// used to derive the application id: the id comes from application.json.
	Filename string
	Data     []byte
	// Actor is the email of the administrator who uploaded it.
	Actor string
}

// PackageMutation is the atomic result of writing one uploaded package.
// Replaced distinguishes a first addition from replacing an existing package
// without a racy list-before-write check in the service layer.
type PackageMutation struct {
	Package
	Replaced bool
}

// PackageCatalog is the writable half of the catalog: the part backed by
// uploaded packages on disk rather than by applications compiled into the binary.
// A server without it still serves its built-in catalog and reports uploads
// unavailable, which is what keeps the feature optional rather than required.
type PackageCatalog interface {
	// Packages lists every stored package, including ones that failed to load
	// — which is the only place the reason a package is missing from the
	// catalog can be reported, so the listing is a view rather than the record.
	Packages() []PackageView
	// AddPackage validates an archive and adds or replaces the catalog entry
	// it carries. It returns the record it stored — what the instances it
	// touches are judged against is the service's to work out. It returns
	// ErrPackageInvalid for a malformed archive and ErrPackageReserved for one
	// whose id belongs to a built-in application.
	AddPackage(upload PackageUpload) (PackageMutation, error)
	// RemovePackage deletes a stored package and its catalog entry.
	RemovePackage(id string) error
}
