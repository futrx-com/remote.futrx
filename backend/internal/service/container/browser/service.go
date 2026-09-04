// Package browser coordinates browser provisioning and runtime transitions for
// project containers.
package browser

import (
	"context"

	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
)

// StackProvisioner prepares the browser packages and runtime assets in a
// container.
type StackProvisioner interface {
	Provision(ctx context.Context, containerName string) error
}

// Runtime controls the already-provisioned browser processes.
type Runtime interface {
	Start(ctx context.Context, containerName string) error
	StartCore(ctx context.Context, containerName string) error
	StartView(ctx context.Context, containerName string) error
	Stop(ctx context.Context, containerName string) error
	StopView(ctx context.Context, containerName string) error
	Running(ctx context.Context, containerName string) (bool, error)
	Status(ctx context.Context, containerName string) (serviceproject.AgentBrowserInfo, error)
}

// Tooling publishes browser assets consumed by shared agent and launch
// preparation.
type Tooling interface {
	EnsureSkill(ctx context.Context, containerName string) error
	EnsureScript(ctx context.Context, containerName string) error
	EnsureMCP(ctx context.Context, containerName string) error
	EnsureNesting(ctx context.Context, containerName string) error
}

// Dependencies groups the independently replaceable browser adapters.
type Dependencies struct {
	Provisioner StackProvisioner
	Runtime     Runtime
	Tooling     Tooling
}

// Service owns the provision-before-start policy and exposes browser
// capabilities to projects plus launch and shared agent preparation.
type Service struct {
	provisioner StackProvisioner
	runtime     Runtime
	tooling     Tooling
	port        int
}

func NewService(deps Dependencies, port int) *Service {
	return &Service{
		provisioner: deps.Provisioner,
		runtime:     deps.Runtime,
		tooling:     deps.Tooling,
		port:        port,
	}
}

// Port returns the in-container noVNC port.
func (s *Service) Port() int { return s.port }

// Ensure provisions the browser stack before starting its core and view.
func (s *Service) Ensure(ctx context.Context, containerName string) error {
	return s.provisionAndStart(ctx, containerName, s.runtime.Start)
}

// EnsureCore provisions the browser stack before starting its shared core.
func (s *Service) EnsureCore(ctx context.Context, containerName string) error {
	return s.provisionAndStart(ctx, containerName, s.runtime.StartCore)
}

// EnsureView provisions the browser stack before starting its noVNC view.
func (s *Service) EnsureView(ctx context.Context, containerName string) error {
	return s.provisionAndStart(ctx, containerName, s.runtime.StartView)
}

func (s *Service) provisionAndStart(
	ctx context.Context,
	containerName string,
	start func(context.Context, string) error,
) error {
	if err := s.provisioner.Provision(ctx, containerName); err != nil {
		return err
	}
	return start(ctx, containerName)
}

func (s *Service) Stop(ctx context.Context, containerName string) error {
	return s.runtime.Stop(ctx, containerName)
}

func (s *Service) StopView(ctx context.Context, containerName string) error {
	return s.runtime.StopView(ctx, containerName)
}

func (s *Service) Running(ctx context.Context, containerName string) (bool, error) {
	return s.runtime.Running(ctx, containerName)
}

func (s *Service) Status(ctx context.Context, containerName string) (serviceproject.AgentBrowserInfo, error) {
	return s.runtime.Status(ctx, containerName)
}

func (s *Service) EnsureSkill(ctx context.Context, containerName string) error {
	return s.tooling.EnsureSkill(ctx, containerName)
}

func (s *Service) EnsureScript(ctx context.Context, containerName string) error {
	return s.tooling.EnsureScript(ctx, containerName)
}

func (s *Service) EnsureMCP(ctx context.Context, containerName string) error {
	return s.tooling.EnsureMCP(ctx, containerName)
}

func (s *Service) EnsureNesting(ctx context.Context, containerName string) error {
	return s.tooling.EnsureNesting(ctx, containerName)
}
