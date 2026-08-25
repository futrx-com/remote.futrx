package applications

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
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
	now       Clock
}

// New builds the applications service. installer/projects may be nil-backed on
// hosts without a container runtime; List/catalog still work.
func New(registry Registry, store Store, installer Installer, projects ProjectContainers, ports PortAllocator) *Service {
	return &Service{
		registry:  registry,
		store:     store,
		installer: installer,
		projects:  projects,
		ports:     ports,
		now:       func() int64 { return time.Now().Unix() },
	}
}

// Catalog returns the installable image catalog.
func (s *Service) Catalog() []Image { return s.registry.List() }

// InstallRequest is a validated user request to install an image.
type InstallRequest struct {
	ImageID      string
	Scope        Scope
	ProjectID    string
	Name         string
	Env          map[string]string
	ExternalPort int    // 0 = auto-allocate
	BindAddress  string // "" = image default
}

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

// Install validates the request, provisions the app in a container, and
// persists the resulting instance.
func (s *Service) Install(ctx context.Context, req InstallRequest) (View, error) {
	if s.installer == nil {
		return View{}, ErrUnavailable
	}
	img, ok := s.registry.Get(req.ImageID)
	if !ok {
		return View{}, ErrUnknownImage
	}
	if !req.Scope.Valid() || !img.SupportsScope(req.Scope) {
		return View{}, ErrScope
	}
	installed, err := s.imageInstalled(ctx, req.Scope, req.ProjectID, img.ID)
	if err != nil {
		return View{}, err
	}
	if installed {
		return View{}, ErrAlreadyInstalled
	}

	id := newInstanceID()
	inst := Instance{
		ID:           id,
		ImageID:      img.ID,
		Name:         displayName(req.Name, img.Name),
		Scope:        req.Scope,
		DeviceName:   "app-" + id,
		InternalPort: img.Port.Internal,
		Protocol:     protoOr(img.Port.Protocol, ProtocolTCP),
		BindAddress:  bindOr(req.BindAddress, img.Port.BindAddress),
		Status:       StatusInstalling,
		CreatedAt:    s.now(),
		UpdatedAt:    s.now(),
	}

	if err := s.resolveContainerTarget(ctx, req, &inst); err != nil {
		return View{}, err
	}

	// Resolve env inputs (apply defaults, generate secrets, enforce required).
	env, err := resolveEnv(img, req.Env)
	if err != nil {
		return View{}, err
	}
	inst.Env = env

	if err := s.allocateHostPort(ctx, req, img, &inst); err != nil {
		return View{}, err
	}

	// Persist as "installing" first so a crash mid-install is recoverable.
	if err := s.store.Put(ctx, inst); err != nil {
		return View{}, err
	}

	if err := s.installer.Install(ctx, InstallSpec{Image: img, Instance: inst}); err != nil {
		_ = s.saveStatus(ctx, &inst, StatusError, err.Error())
		return View{}, err
	}
	if err := s.saveStatus(ctx, &inst, StatusRunning, ""); err != nil {
		return View{}, err
	}
	return s.view(inst), nil
}

// resolveContainerTarget picks the container an instance runs in: a project's
// container (readied first) for project scope, or a dedicated one for global.
func (s *Service) resolveContainerTarget(ctx context.Context, req InstallRequest, inst *Instance) error {
	switch req.Scope {
	case ScopeProject:
		if req.ProjectID == "" {
			return ErrProjectneeded
		}
		if s.projects == nil {
			return ErrUnavailable
		}
		name, err := s.projects.ContainerName(ctx, req.ProjectID)
		if err != nil {
			return err
		}
		if err := s.projects.EnsureRunning(ctx, req.ProjectID); err != nil {
			return err
		}
		inst.ProjectID = req.ProjectID
		inst.ContainerName = name
	case ScopeGlobal:
		inst.ContainerName = appContainerName(inst.ID)
	}
	return nil
}

// allocateHostPort assigns a free host port, preferring the requested port,
// then the image default, then the internal port.
func (s *Service) allocateHostPort(ctx context.Context, req InstallRequest, img Image, inst *Instance) error {
	preferred := req.ExternalPort
	if preferred == 0 {
		preferred = img.Port.DefaultExternal
	}
	if preferred == 0 {
		preferred = img.Port.Internal
	}
	if req.ExternalPort != 0 && (req.ExternalPort < 1 || req.ExternalPort > 65535) {
		return ErrPortRange
	}
	taken, err := s.reservedPorts(ctx)
	if err != nil {
		return err
	}
	port, err := s.ports.Allocate(ctx, inst.BindAddress, preferred, taken)
	if err != nil {
		return err
	}
	inst.ExternalPort = port
	return nil
}

// saveStatus stamps status/error/updatedAt on an instance and persists it.
func (s *Service) saveStatus(ctx context.Context, inst *Instance, status InstanceStatus, errMsg string) error {
	inst.Status = status
	inst.Error = errMsg
	inst.UpdatedAt = s.now()
	return s.store.Put(ctx, *inst)
}

// Start starts a stopped instance.
func (s *Service) Start(ctx context.Context, id string) (View, error) {
	return s.transition(ctx, id, func(spec InstallSpec) error {
		return s.installer.Start(ctx, spec)
	}, StatusRunning)
}

// Stop stops a running instance and releases its host port.
func (s *Service) Stop(ctx context.Context, id string) (View, error) {
	return s.transition(ctx, id, func(spec InstallSpec) error {
		return s.installer.Stop(ctx, spec)
	}, StatusStopped)
}

// SetPort changes the external host port of an instance.
func (s *Service) SetPort(ctx context.Context, id string, port int) (View, error) {
	if port < 1 || port > 65535 {
		return View{}, ErrPortRange
	}
	inst, img, err := s.load(ctx, id)
	if err != nil {
		return View{}, err
	}
	if port != inst.ExternalPort {
		taken, err := s.reservedPorts(ctx)
		if err != nil {
			return View{}, err
		}
		delete(taken, inst.ExternalPort) // our own current port is fine to keep
		if taken[port] {
			return View{}, fmt.Errorf("%w: port %d already in use", ErrPortRange, port)
		}
		inst.ExternalPort = port
	}
	inst.UpdatedAt = s.now()
	spec := InstallSpec{Image: img, Instance: inst}
	if err := s.installer.Expose(ctx, spec); err != nil {
		return View{}, err
	}
	if err := s.store.Put(ctx, inst); err != nil {
		return View{}, err
	}
	return s.view(inst), nil
}

// Uninstall removes an instance and its container-side footprint.
func (s *Service) Uninstall(ctx context.Context, id string) error {
	if s.installer == nil {
		return ErrUnavailable
	}
	inst, img, err := s.load(ctx, id)
	if err != nil {
		return err
	}
	if err := s.installer.Uninstall(ctx, InstallSpec{Image: img, Instance: inst}); err != nil {
		return err
	}
	return s.store.Delete(ctx, id)
}

// transition runs a lifecycle action and records the resulting status.
func (s *Service) transition(ctx context.Context, id string, action func(InstallSpec) error, target InstanceStatus) (View, error) {
	if s.installer == nil {
		return View{}, ErrUnavailable
	}
	inst, img, err := s.load(ctx, id)
	if err != nil {
		return View{}, err
	}
	if inst.Scope == ScopeProject && s.projects != nil && target == StatusRunning {
		if err := s.projects.EnsureRunning(ctx, inst.ProjectID); err != nil {
			return View{}, err
		}
	}
	if err := action(InstallSpec{Image: img, Instance: inst}); err != nil {
		_ = s.saveStatus(ctx, &inst, StatusError, err.Error())
		return View{}, err
	}
	if err := s.saveStatus(ctx, &inst, target, ""); err != nil {
		return View{}, err
	}
	return s.view(inst), nil
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

// imageInstalled reports whether an image is already installed in the given
// scope (globally, or within the given project) — enforcing one instance per
// image per scope.
func (s *Service) imageInstalled(ctx context.Context, scope Scope, projectID, imageID string) (bool, error) {
	var (
		list []Instance
		err  error
	)
	if scope == ScopeGlobal {
		list, err = s.store.ListGlobal(ctx)
	} else {
		list, err = s.store.ListProject(ctx, projectID)
	}
	if err != nil {
		return false, err
	}
	for _, in := range list {
		if in.ImageID == imageID {
			return true, nil
		}
	}
	return false, nil
}

// reservedPorts is the set of host ports already claimed by other instances.
// Every instance (global and project) binds a host port, so the conflict domain
// is the whole server regardless of the new instance's scope.
func (s *Service) reservedPorts(ctx context.Context) (map[int]bool, error) {
	taken := map[int]bool{}
	all, err := s.store.ListAll(ctx)
	if err != nil {
		return nil, err
	}
	for _, in := range all {
		if in.ExternalPort > 0 {
			taken[in.ExternalPort] = true
		}
	}
	return taken, nil
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

// ---- helpers ---------------------------------------------------------------

// appContainerPrefix names dedicated global-app containers: futrx-app-<id>.
const appContainerPrefix = "futrx-app-"

func appContainerName(id string) string {
	// id is 12 hex chars, so the name stays well within LXD's 63-char limit.
	return appContainerPrefix + id
}

func newInstanceID() string {
	var b [6]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func displayName(requested, fallback string) string {
	requested = strings.TrimSpace(requested)
	if requested == "" {
		return fallback
	}
	return requested
}

func protoOr(p, def Protocol) Protocol {
	if p == "" {
		return def
	}
	return p
}

func bindOr(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	if strings.TrimSpace(b) != "" {
		return b
	}
	return "127.0.0.1"
}

func secretKeys(img Image) map[string]bool {
	m := map[string]bool{}
	for _, e := range img.Env {
		if e.Secret {
			m[e.Key] = true
		}
	}
	return m
}

// resolveEnv applies defaults, generates secrets, and enforces required inputs.
func resolveEnv(img Image, provided map[string]string) (map[string]string, error) {
	out := map[string]string{}
	for _, e := range img.Env {
		v := strings.TrimSpace(provided[e.Key])
		if v == "" {
			v = e.Default
		}
		if v == "" && e.Generate == "password" {
			v = generatePassword()
		}
		if v == "" && e.Required {
			return nil, fmt.Errorf("%w: %s", ErrRequiredEnv, e.Key)
		}
		if v != "" {
			out[e.Key] = v
		}
	}
	return out, nil
}

const passwordAlphabet = "abcdefghijkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"

func generatePassword() string {
	const n = 24
	b := make([]byte, n)
	_, _ = rand.Read(b)
	out := make([]byte, n)
	for i := range b {
		out[i] = passwordAlphabet[int(b[i])%len(passwordAlphabet)]
	}
	return string(out)
}
