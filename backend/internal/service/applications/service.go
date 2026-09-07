package applications

import (
	"context"
	"errors"
	"time"
)

// Errors returned by the service. Handlers map these to HTTP status codes.
var (
	ErrUnavailable      = errors.New("applications: container runtime unavailable")
	ErrUnknownImage     = errors.New("applications: unknown image")
	ErrScope            = errors.New("applications: image does not support this scope")
	ErrProjectneeded    = errors.New("applications: project id required")
	ErrRequiredEnv      = errors.New("applications: missing required value")
	ErrNotFound         = errors.New("applications: instance not found")
	ErrPortRange        = errors.New("applications: external port out of range")
	ErrAlreadyInstalled = errors.New("applications: this image is already installed in this scope")
	ErrNotSupported     = errors.New("applications: not supported for this image type")

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
	registry  Registry
	store     Store
	installer Installer
	projects  ProjectContainers
	ports     PortAllocator
	backends  BackendHost
	packages  PackageCatalog
	now       Clock
}

// Option configures optional service dependencies. Backend plugin hosting is
// optional because a server without a Go toolchain, or a build that ships no
// plugin images, must still install and run everything else.
type Option func(*Service)

// WithBackendHost enables images that ship a plugin/ directory. Without it,
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
	return service
}

// Catalog returns the installable image catalog.
func (s *Service) Catalog() []Image { return s.registry.List() }

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
// image's Connection descriptor. The transport layer authorizes the caller.
func (s *Service) Credentials(ctx context.Context, id string) (Credentials, error) {
	inst, img, err := s.load(ctx, id)
	if err != nil {
		return Credentials{}, err
	}
	conn := img.Connection
	username := conn.User
	if conn.UserEnv != "" {
		username = inst.Env[conn.UserEnv]
	}
	return Credentials{
		ContainerName: inst.ContainerName,
		LXDHost:       inst.ContainerName + ".lxd",
		InternalPort:  inst.InternalPort,
		ExternalPort:  inst.ExternalPort,
		BindAddress:   inst.BindAddress,
		Username:      username,
		Password:      inst.Env[conn.PasswordEnv],
		Database:      inst.Env[conn.DatabaseEnv],
		Env:           inst.Env,
	}, nil
}

// saveStatus stamps status/error/updatedAt on an instance and persists it.
func (s *Service) saveStatus(ctx context.Context, inst *Instance, status InstanceStatus, errMsg string) error {
	inst.Status = status
	inst.Error = errMsg
	inst.UpdatedAt = s.now()
	return s.store.Put(ctx, *inst)
}

func (s *Service) load(ctx context.Context, id string) (Instance, Image, error) {
	inst, ok, err := s.store.Get(ctx, id)
	if err != nil {
		return Instance{}, Image{}, err
	}
	if !ok {
		return Instance{}, Image{}, ErrNotFound
	}
	img, ok := s.registry.Get(inst.ImageID)
	if !ok {
		return Instance{}, Image{}, ErrUnknownImage
	}
	return inst, img, nil
}

// view / views project Instances to API-safe Views (secret env redacted).
func (s *Service) view(inst Instance) View {
	img, _ := s.registry.Get(inst.ImageID)
	pub := map[string]string{}
	secret := secretKeys(img)
	for k, v := range inst.Env {
		if !secret[k] {
			pub[k] = v
		}
	}
	safe := inst
	safe.Env = nil // never leak secrets through the Instance blob
	return View{Instance: safe, EnvPublic: pub}
}

func (s *Service) views(insts []Instance) []View {
	out := make([]View, 0, len(insts))
	for _, in := range insts {
		out = append(out, s.view(in))
	}
	return out
}
