package applications

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
)

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

// Install validates the request, provisions the app in a container, and
// persists the resulting instance.
func (s *Service) Install(ctx context.Context, req InstallRequest) (View, error) {
	img, ok := s.registry.Get(req.ImageID)
	if !ok {
		return View{}, ErrUnknownImage
	}
	if !req.Scope.Valid() || !img.SupportsScope(req.Scope) {
		return View{}, ErrScope
	}
	if err := s.claimInstallSlot(ctx, req.Scope, req.ProjectID, img.ID); err != nil {
		return View{}, err
	}

	id := newInstanceID()
	inst := Instance{
		ID:           id,
		ImageID:      img.ID,
		ImageVersion: img.Version,
		Name:         displayName(req.Name, img.Name),
		Scope:        req.Scope,
		ProjectID:    req.ProjectID,
		Status:       StatusInstalling,
		CreatedAt:    s.now(),
		UpdatedAt:    s.now(),
	}

	// Resolve env inputs (apply defaults, generate secrets, enforce required).
	env, err := resolveEnv(img, req.Env)
	if err != nil {
		return View{}, err
	}
	inst.Env = env

	if s.installer == nil {
		return View{}, ErrUnavailable
	}
	// A tool is provisioned into a container but exposes nothing, so it gets no
	// device name, no internal port, and no host port: there is nothing for a
	// proxy to forward.
	if img.Type.NeedsPort() {
		inst.DeviceName = "app-" + id
		inst.InternalPort = img.Port.Internal
		inst.Protocol = protoOr(img.Port.Protocol, ProtocolTCP)
		inst.BindAddress = bindOr(req.BindAddress, img.Port.BindAddress)
	}

	if err := s.resolveContainerTarget(ctx, req, &inst); err != nil {
		return View{}, err
	}
	if img.Type.NeedsPort() {
		if err := s.allocateHostPort(ctx, req, img, &inst); err != nil {
			return View{}, err
		}
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

// claimInstallSlot enforces one instance per image per scope, and decides what
// installing over an existing one means.
//
// An instance whose install failed is not an installation: it is the record of
// an attempt, kept so its error and whatever it left in a container stay
// visible and inspectable. Refusing to install over it would leave the user
// with a row they cannot act on except by uninstalling it by hand — so
// installing again is treated as a retry, and the failed attempt is torn down
// here exactly as an uninstall would tear it down.
//
// Installing over a *working* instance is still refused. That is the invariant
// the rest of the system reads: one running copy per image per scope, so an
// extension's identity and its backend are unambiguous.
func (s *Service) claimInstallSlot(ctx context.Context, scope Scope, projectID, imageID string) error {
	existing, found, err := s.instanceOfImage(ctx, scope, projectID, imageID)
	if err != nil || !found {
		return err
	}
	if existing.Status != StatusError {
		return ErrAlreadyInstalled
	}
	img, ok := s.registry.Get(existing.ImageID)
	if !ok {
		return ErrUnknownImage
	}
	if err := s.teardown(ctx, img, existing); err != nil {
		return err
	}
	return s.store.Delete(ctx, existing.ID)
}

// instanceOfImage returns the instance of an image in the given scope
// (globally, or within the given project), if there is one.
func (s *Service) instanceOfImage(ctx context.Context, scope Scope, projectID, imageID string) (Instance, bool, error) {
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
		return Instance{}, false, err
	}
	for _, in := range list {
		if in.ImageID == imageID {
			return in, true, nil
		}
	}
	return Instance{}, false, nil
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
