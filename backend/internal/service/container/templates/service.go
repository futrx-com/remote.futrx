package templates

import (
	"context"
	"log"
	"path"
	"sync"
	"time"

	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
	"github.com/futrx-com/remote.futrx.com/internal/shared/output"
)

// provisionTimeout bounds one provisioning run. A WordPress or Laravel stack
// on a small host is minutes of apt and composer work, so the budget is
// generous; it exists to stop a wedged run from leaking a goroutine forever.
const provisionTimeout = 45 * time.Minute

// probeTimeout bounds the cheap marker/seed probes on the hot path (every
// project start pays one of them for a provisioning template).
const probeTimeout = 15 * time.Second

var _ serviceproject.ContainerTemplates = (*Service)(nil)

// Service applies templates to project containers. Provisioning runs in the
// background because it can take many minutes; callers observe progress
// through Status, which the project status payload surfaces.
type Service struct {
	catalog *Catalog
	runtime Runtime
	timeout time.Duration

	mu     sync.Mutex
	states map[string]State
}

// NewService returns a template service backed by a container runtime.
func NewService(catalog *Catalog, runtime Runtime) *Service {
	return &Service{
		catalog: catalog,
		runtime: runtime,
		timeout: provisionTimeout,
		states:  make(map[string]State),
	}
}

// Catalog exposes the immutable template set.
func (s *Service) Catalog() *Catalog { return s.catalog }

// Has reports whether name is a known template.
func (s *Service) Has(name string) bool { return s.catalog.Has(name) }

// DefaultName returns the template assigned when a caller requests none.
func (s *Service) DefaultName() string { return s.catalog.DefaultName() }

// List returns the catalog annotated with per-host availability of each
// template's dedicated pre-built image.
func (s *Service) List(ctx context.Context) []Descriptor {
	templates := s.catalog.List()
	out := make([]Descriptor, 0, len(templates))
	for _, template := range templates {
		descriptor := Descriptor{
			Name:          template.Name,
			Title:         template.Title,
			Description:   template.Description,
			Icon:          template.Icon,
			DefaultPorts:  template.DefaultPorts,
			Default:       template.Name == DefaultName,
			Provisions:    template.Provisions(),
			PrebuiltImage: template.ImageAlias(),
		}
		if descriptor.PrebuiltImage != "" {
			descriptor.PrebuiltImageAvailable = s.imagePublished(ctx, descriptor.PrebuiltImage)
		}
		out = append(out, descriptor)
	}
	return out
}

// ImageFor returns the dedicated image alias a template's containers should
// launch from, or an empty string when the caller must fall back to the shared
// base image plus a provision script.
func (s *Service) ImageFor(ctx context.Context, name string) string {
	alias := s.catalog.Resolve(name).ImageAlias()
	if alias == "" || !s.imagePublished(ctx, alias) {
		return ""
	}
	return alias
}

func (s *Service) imagePublished(ctx context.Context, alias string) bool {
	if s.runtime == nil || !s.runtime.Available() {
		return false
	}
	pctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	published, err := s.runtime.ImageExists(pctx, alias)
	if err != nil {
		log.Printf("templates: probe image %s: %v", alias, err)
		return false
	}
	return published
}

// Ensure provisions a container for its template unless the marker file says a
// previous run (or a pre-built template image) already did. It returns a
// channel closed once any background work has settled; the lifecycle ignores
// it, tests and future callers can wait on it.
//
// Ensure is called on every convergence of a project container, not only on
// creation: the container rootfs is disposable, so a replaced container must
// be re-provisioned, and an interrupted run must be retried.
func (s *Service) Ensure(ctx context.Context, containerName, name string) <-chan struct{} {
	done := make(chan struct{})
	template := s.catalog.Resolve(name)
	if !template.Provisions() || s.runtime == nil || !s.runtime.Available() {
		close(done)
		return done
	}

	observation := s.observe(ctx, containerName)
	observation.InFlight = s.inFlight(containerName)
	decision := Decide(template, observation)
	if !decision.Run {
		s.recordObserved(containerName, template, observation)
		close(done)
		return done
	}

	s.begin(containerName, template)
	// Provisioning deliberately outlives the request that triggered it: the
	// caller is a container start, which must not block for minutes.
	go func() {
		defer close(done)
		runCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), s.timeout)
		defer cancel()
		s.finish(containerName, template, s.provision(runCtx, containerName, template))
	}()
	return done
}

// provision writes the seeds and runs the provision program. Seeding happens
// first so the agent instructions are in place even if the install fails.
func (s *Service) provision(ctx context.Context, containerName string, template Template) error {
	for _, seed := range template.allSeeds() {
		if err := s.seed(ctx, containerName, seed); err != nil {
			return err
		}
	}
	program := ProvisionProgram(template)
	if program == "" {
		return nil
	}
	if out, err := s.runtime.RunScript(ctx, containerName, program); err != nil {
		return provisionError{
			template: template.Name,
			cause:    err,
			output:   output.TruncateTail(out, 2000),
		}
	}
	return nil
}

// seed writes one file, never overwriting an existing one: /workspace is
// durable and may already hold the user's own version of that path.
func (s *Service) seed(ctx context.Context, containerName string, seed Seed) error {
	exists, err := s.runtime.FileExists(ctx, containerName, seed.Target)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	if err := s.runtime.EnsureDirectory(ctx, containerName, path.Dir(seed.Target)); err != nil {
		return err
	}
	return s.runtime.PushFile(ctx, containerName, seed.Content, seed.Target, seed.Mode)
}

func (s *Service) observe(ctx context.Context, containerName string) Observation {
	pctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	marker, err := s.runtime.FileExists(pctx, containerName, MarkerPath)
	if err != nil {
		log.Printf("templates: probe marker in %s: %v", containerName, err)
	}
	failure, err := s.runtime.FileExists(pctx, containerName, FailurePath)
	if err != nil {
		log.Printf("templates: probe failure marker in %s: %v", containerName, err)
	}
	return Observation{MarkerPresent: marker, FailurePresent: failure}
}

// Status reports one container's template provisioning state. In-memory state
// from a run this process started wins; otherwise the container's marker files
// are the source of truth, so the answer survives a backend restart.
func (s *Service) Status(ctx context.Context, containerName, name string) State {
	template := s.catalog.Resolve(name)
	base := State{Template: template.Name, Title: template.Title, LogPath: LogPath}
	if !template.Provisions() {
		base.Status = StatusNone
		base.LogPath = ""
		return base
	}
	if state, ok := s.state(containerName); ok && state.Template == template.Name {
		return state
	}
	if s.runtime == nil || !s.runtime.Available() {
		base.Status = StatusPending
		return base
	}
	base.Status = ObservedStatus(template, s.observe(ctx, containerName))
	return base
}

// TemplateStatus adapts Status to the project status payload.
func (s *Service) TemplateStatus(
	ctx context.Context,
	containerName, name string,
) serviceproject.TemplateStatus {
	state := s.Status(ctx, containerName, name)
	return serviceproject.TemplateStatus{
		Name:       state.Template,
		Title:      state.Title,
		Status:     string(state.Status),
		Error:      state.Error,
		LogPath:    state.LogPath,
		StartedAt:  state.StartedAt,
		FinishedAt: state.FinishedAt,
	}
}

func (s *Service) state(containerName string) (State, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, ok := s.states[containerName]
	return state, ok
}

func (s *Service) inFlight(containerName string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.states[containerName].Status == StatusRunning
}

func (s *Service) begin(containerName string, template Template) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.states[containerName] = State{
		Template:  template.Name,
		Title:     template.Title,
		Status:    StatusRunning,
		LogPath:   LogPath,
		StartedAt: time.Now().UnixMilli(),
	}
}

func (s *Service) finish(containerName string, template Template, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.states[containerName]
	state.Template = template.Name
	state.Title = template.Title
	state.LogPath = LogPath
	state.FinishedAt = time.Now().UnixMilli()
	if err != nil {
		state.Status = StatusFailed
		state.Error = err.Error()
		log.Printf("templates: provision %s in %s: %v", template.Name, containerName, err)
	} else {
		state.Status = StatusDone
		state.Error = ""
	}
	s.states[containerName] = state
}

func (s *Service) recordObserved(containerName string, template Template, observation Observation) {
	status := ObservedStatus(template, observation)
	s.mu.Lock()
	defer s.mu.Unlock()
	existing, ok := s.states[containerName]
	if ok && existing.Template == template.Name && existing.Status == status {
		return
	}
	s.states[containerName] = State{
		Template: template.Name,
		Title:    template.Title,
		Status:   status,
		LogPath:  LogPath,
	}
}

// ForgetTemplateState drops cached state for a container that no longer
// exists, so a slug reused by a later project starts from a clean answer.
func (s *Service) ForgetTemplateState(containerName string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.states, containerName)
}

type provisionError struct {
	template string
	cause    error
	output   string
}

func (e provisionError) Error() string {
	message := "provision template " + e.template + ": " + e.cause.Error()
	if e.output != "" {
		message += "; output: " + e.output
	}
	return message
}

func (e provisionError) Unwrap() error { return e.cause }
