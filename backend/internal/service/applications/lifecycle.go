package applications

import (
	"context"
	"fmt"
)

// Start starts a stopped instance.
func (s *Service) Start(ctx context.Context, id string) (View, error) {
	return s.transition(ctx, id, StatusRunning)
}

// Stop stops a running instance and releases its host port.
func (s *Service) Stop(ctx context.Context, id string) (View, error) {
	return s.transition(ctx, id, StatusStopped)
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
	if !img.Type.NeedsContainer() {
		return View{}, fmt.Errorf("%w: %s has no port", ErrNotSupported, img.ID)
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

// Uninstall removes an instance and everything it left behind.
func (s *Service) Uninstall(ctx context.Context, id string) error {
	inst, img, err := s.load(ctx, id)
	if err != nil {
		return err
	}
	if err := s.teardown(ctx, img, inst); err != nil {
		return err
	}
	return s.store.Delete(ctx, id)
}

// teardown removes an instance's footprint: its container side, and its plugin
// process and data. Uninstalling and retrying a failed install share it, so
// both leave exactly the same state behind.
func (s *Service) teardown(ctx context.Context, img Image, inst Instance) error {
	if img.Type.NeedsContainer() {
		if s.installer == nil {
			return ErrUnavailable
		}
		if err := s.installer.Uninstall(ctx, InstallSpec{Image: img, Instance: inst}); err != nil {
			return err
		}
	}
	return s.removeBackend(ctx, img, inst)
}

// transition runs a lifecycle action and records the resulting status.
func (s *Service) transition(ctx context.Context, id string, target InstanceStatus) (View, error) {
	inst, img, err := s.load(ctx, id)
	if err != nil {
		return View{}, err
	}
	// Start/stop on a UI image is purely a record: "stopped" means the SPA
	// stops loading its extension, which is the whole effect it can have. A
	// backend image has one real effect — its plugin process — so the record
	// and the process move together.
	if !img.Type.NeedsContainer() {
		if err := s.moveBackend(ctx, img, inst, target); err != nil {
			_ = s.saveStatus(ctx, &inst, StatusError, err.Error())
			return View{}, err
		}
		if err := s.saveStatus(ctx, &inst, target, ""); err != nil {
			return View{}, err
		}
		return s.view(inst), nil
	}
	if s.installer == nil {
		return View{}, ErrUnavailable
	}
	if inst.Scope == ScopeProject && s.projects != nil && target == StatusRunning {
		if err := s.projects.EnsureRunning(ctx, inst.ProjectID); err != nil {
			return View{}, err
		}
	}
	if err := s.moveContainer(ctx, InstallSpec{Image: img, Instance: inst}, target); err != nil {
		_ = s.saveStatus(ctx, &inst, StatusError, err.Error())
		return View{}, err
	}
	if err := s.moveBackend(ctx, img, inst, target); err != nil {
		_ = s.saveStatus(ctx, &inst, StatusError, err.Error())
		return View{}, err
	}
	if err := s.saveStatus(ctx, &inst, target, ""); err != nil {
		return View{}, err
	}
	return s.view(inst), nil
}

// moveContainer applies the requested lifecycle state to the container half
// of an image. The transition workflow deliberately runs this before moving a
// plugin and recording the final status.
func (s *Service) moveContainer(ctx context.Context, spec InstallSpec, target InstanceStatus) error {
	if target == StatusRunning {
		return s.installer.Start(ctx, spec)
	}
	return s.installer.Stop(ctx, spec)
}
