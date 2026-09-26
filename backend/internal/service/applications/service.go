package applications

import (
	"context"
	"errors"
	"fmt"
	"time"
)

const maxInstanceErrorRunes = 4000

// Errors returned by the service. Handlers map these to HTTP status codes.
var (
	ErrUnavailable        = errors.New("applications: container runtime unavailable")
	ErrUnknownApplication = errors.New("applications: unknown application")
	ErrScope              = errors.New("applications: application does not support this scope")
	ErrProjectneeded      = errors.New("applications: project id required")
	ErrRequiredEnv        = errors.New("applications: missing required value")
	ErrInvalidEnv         = errors.New("applications: invalid setting value")
	ErrNotFound           = errors.New("applications: instance not found")
	ErrPortRange          = errors.New("applications: external port out of range")
	ErrAlreadyInstalled   = errors.New("applications: this application is already installed in this scope")
	ErrNotSupported       = errors.New("applications: capability not supported")
	ErrInvalidState       = errors.New("applications: invalid lifecycle state")

	// Uploaded-package errors.
	ErrPackagesUnavailable = errors.New("applications: uploaded packages are not available on this server")
	ErrPackageInvalid      = errors.New("applications: invalid application package")
	ErrPackageReserved     = errors.New("applications: an application with this id is built into this server")
	// ErrPackageSuperseded is the same rule seen from the other side: not an
	// upload refused, but files already stored when a later release built that
	// application into the binary. The server serves the built-in one and the
	// upload is inert, which is a thing to clean up rather than a failure.
	ErrPackageSuperseded = errors.New(
		"applications: this application is now built into the server, so the uploaded copy is unused and can be removed")
	ErrPackageNotFound = errors.New("applications: package not found")
	ErrPackageInUse    = errors.New("applications: uninstall this application everywhere before removing its package")
)

// Clock returns the current unix time; injectable for tests.
type Clock func() int64

// Service is the policy layer for installable applications.
type Service struct {
	registry      Registry
	store         Store
	installer     Installer
	projects      ProjectContainers
	ports         PortAllocator
	backends      BackendHost
	packages      PackageCatalog
	lifecycle     ApplicationLifecyclePublisher
	eventSource   EventSource
	eventContext  context.Context
	eventRouter   *applicationEventRouter
	instanceLocks instanceLockSet
	now           Clock
}

// Option configures optional service dependencies. Backend backend hosting is
// optional because a server without a Go toolchain, or a build that ships no
// backend applications, must still install and run everything else.
type Option func(*Service)

// WithBackendHost enables applications that ship a host backend. Without it,
// their catalog entries still load and every backend call reports the feature
// unavailable.
func WithBackendHost(host BackendHost) Option {
	return func(s *Service) {
		if host != nil {
			s.backends = host
		}
	}
}

// WithPackageCatalog enables uploading application packages. Without it the
// catalog is exactly what the binary was built with, and the package routes
// report the feature unavailable — which is the right answer for a server
// whose state directory is not writable.
func WithPackageCatalog(packages PackageCatalog) Option {
	return func(s *Service) {
		if packages != nil {
			s.packages = packages
		}
	}
}

// WithLifecyclePublisher reports successful catalog and installed-copy
// transitions. It is optional so the applications service remains usable in
// isolated tools and tests that have no process-wide lifecycle composition.
func WithLifecyclePublisher(publisher ApplicationLifecyclePublisher) Option {
	return func(s *Service) {
		if publisher != nil {
			s.lifecycle = publisher
		}
	}
}

// WithEventSource routes process-wide events to running application backends.
// ctx owns the subscription and worker lifetime; cancellation unsubscribes and
// discards any events still queued during process shutdown.
func WithEventSource(ctx context.Context, source EventSource) Option {
	return func(s *Service) {
		if source != nil {
			s.eventContext = ctx
			s.eventSource = source
		}
	}
}

// New builds the applications service. installer/projects may be nil-backed on
// hosts without a container runtime; List/catalog still work.
func New(
	registry Registry,
	store Store,
	installer Installer,
	projects ProjectContainers,
	ports PortAllocator,
	options ...Option,
) *Service {
	service := &Service{
		registry:  registry,
		store:     store,
		installer: installer,
		projects:  projects,
		ports:     ports,
		now:       func() int64 { return time.Now().Unix() },
	}
	for _, option := range options {
		option(service)
	}
	service.startEventRouter()
	return service
}

// Catalog returns the installable application catalog.
func (s *Service) Catalog() []Application { return s.registry.List() }

// ListGlobal returns installed global apps as API-safe views.
func (s *Service) ListGlobal(ctx context.Context) ([]View, error) {
	insts, err := s.store.ListGlobal(ctx)
	if err != nil {
		return nil, err
	}
	return s.views(insts), nil
}

// ListProject returns a project's installed apps as API-safe views.
func (s *Service) ListProject(ctx context.Context, projectID string) ([]View, error) {
	insts, err := s.store.ListProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	return s.views(insts), nil
}

// Get returns a single instance as an API-safe view.
func (s *Service) Get(ctx context.Context, id string) (View, bool, error) {
	inst, ok, err := s.store.Get(ctx, id)
	if err != nil || !ok {
		return View{}, ok, err
	}
	return s.view(inst), true, nil
}

// Credentials returns full connection details for an instance, including secret
// env values and the canonical user/password/database resolved from the
// application's Connection descriptor. The transport layer authorizes the caller.
func (s *Service) Credentials(ctx context.Context, id string) (Credentials, error) {
	inst, application, err := s.load(ctx, id)
	if err != nil {
		return Credentials{}, err
	}
	env := declaredEnv(application, inst.Env)
	conn := application.Connection
	username := conn.User
	if conn.UserEnv != "" {
		username = env[conn.UserEnv]
	}
	return Credentials{
		ContainerName: inst.ContainerName,
		LXDHost:       inst.ContainerName + ".lxd",
		InternalPort:  inst.InternalPort,
		ExternalPort:  inst.ExternalPort,
		BindAddress:   inst.BindAddress,
		Username:      username,
		Password:      env[conn.PasswordEnv],
		Database:      env[conn.DatabaseEnv],
		Env:           env,
	}, nil
}

// saveStatus stamps status/error/updatedAt on an instance and persists it.
func (s *Service) saveStatus(ctx context.Context, inst *Instance, status InstanceStatus, errMsg string) error {
	inst.Status = status
	inst.Error = boundedInstanceError(errMsg)
	inst.UpdatedAt = s.now()
	return s.store.Put(ctx, *inst)
}

func (s *Service) load(ctx context.Context, id string) (Instance, Application, error) {
	inst, ok, err := s.store.Get(ctx, id)
	if err != nil {
		return Instance{}, Application{}, err
	}
	if !ok {
		return Instance{}, Application{}, ErrNotFound
	}
	application, ok := s.registry.Get(inst.ApplicationID)
	if !ok {
		return Instance{}, Application{}, ErrUnknownApplication
	}
	return inst, application, nil
}

// view / views project Instances to API-safe Views (secret env redacted).
func (s *Service) view(inst Instance) View {
	application, _ := s.registry.Get(inst.ApplicationID)
	// Old records may predate the write-side bound. Never make an applications
	// page carry an arbitrarily large command dump just because one is still on
	// disk; the retained prefix and tail preserve the useful failure context.
	inst.Error = boundedInstanceError(inst.Error)
	pub := map[string]string{}
	for _, variable := range application.Env {
		if !variable.Secret {
			if value, ok := inst.Env[variable.Key]; ok {
				pub[variable.Key] = value
			}
		}
	}
	safe := inst
	safe.Env = nil // never leak secrets through the Instance blob
	return View{Instance: safe, EnvPublic: pub}
}

func declaredEnv(application Application, stored map[string]string) map[string]string {
	declared := make(map[string]string, len(application.Env))
	for _, variable := range application.Env {
		if value, ok := stored[variable.Key]; ok {
			declared[variable.Key] = value
		}
	}
	return declared
}

func boundedInstanceError(message string) string {
	runes := []rune(message)
	if len(runes) <= maxInstanceErrorRunes {
		return message
	}
	const (
		prefixRunes = 1200
		markerRoom  = 64
	)
	tailRunes := maxInstanceErrorRunes - prefixRunes - markerRoom
	omitted := len(runes) - prefixRunes - tailRunes
	marker := []rune(fmt.Sprintf("\n… %d characters omitted …\n", omitted))
	return string(runes[:prefixRunes]) + string(marker) + string(runes[len(runes)-tailRunes:])
}

func (s *Service) views(insts []Instance) []View {
	out := make([]View, 0, len(insts))
	for _, in := range insts {
		out = append(out, s.view(in))
	}
	return out
}
