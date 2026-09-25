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
	unlock := s.instanceLocks.lock(id)
	defer unlock()

	inst, application, err := s.load(ctx, id)
	if err != nil {
		return View{}, err
	}
	if !application.NeedsPort() {
		return View{}, fmt.Errorf("%w: %s has no port", ErrNotSupported, application.ID)
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
	spec := InstallSpec{Application: application, Instance: inst}
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
	unlock := s.instanceLocks.lock(id)
	defer unlock()
	inst, err := s.uninstallLocked(ctx, id)
	if err != nil {
		return err
	}
	s.publishApplicationUninstalled(ctx, inst)
	return nil
}

// uninstallLocked tears down one installed copy while its instance lock is
// held. The caller publishes before releasing the lock so committed lifecycle
// events preserve the same per-instance order as their state transitions.
func (s *Service) uninstallLocked(ctx context.Context, id string) (Instance, error) {
	inst, application, err := s.load(ctx, id)
	if err != nil {
		return Instance{}, err
	}
	if err := s.teardown(ctx, application, inst); err != nil {
		return Instance{}, err
	}
	if err := s.store.Delete(ctx, id); err != nil {
		return Instance{}, err
	}
	return inst, nil
}

// teardown removes an instance's footprint: its container side, and its backend
// process and data. Uninstalling and retrying a failed install share it, so
// both leave exactly the same state behind.
func (s *Service) teardown(ctx context.Context, application Application, inst Instance) error {
	if application.NeedsContainer() {
		if s.installer == nil {
			return ErrUnavailable
		}
		if err := s.installer.Uninstall(ctx, InstallSpec{Application: application, Instance: inst}); err != nil {
			return err
		}
	}
	return s.removeBackend(ctx, application, inst)
}

// transition runs a lifecycle action and records the resulting status.
func (s *Service) transition(ctx context.Context, id string, target InstanceStatus) (View, error) {
	unlock := s.instanceLocks.lock(id)
	defer unlock()
	view, inst, previousStatus, err := s.transitionLocked(ctx, id, target)
	if err != nil {
		return View{}, err
	}
	s.publishApplicationTransition(ctx, inst, previousStatus, target)
	return view, nil
}

// transitionLocked performs the state change while its instance lock is held.
// The returned event data is published while the caller still holds it.
func (s *Service) transitionLocked(
	ctx context.Context,
	id string,
	target InstanceStatus,
) (View, Instance, InstanceStatus, error) {
	inst, application, err := s.load(ctx, id)
	if err != nil {
		return View{}, Instance{}, "", err
	}
	// Error records are failed install attempts, not stopped installations. They
	// can only be retried through Install (which first tears down the partial
	// attempt) or explicitly uninstalled. Treating one as stopped can hand an
	// empty legacy container name to LXD and, more importantly, skips the
	// cleanup-and-recreate contract of Retry.
	if inst.Status == StatusError || inst.Status == StatusInstalling {
		return View{}, Instance{}, "", fmt.Errorf(
			"%w: %s instances must be retried or uninstalled", ErrInvalidState, inst.Status)
	}
	previousStatus := inst.Status
	// An application without infrastructure may be purely a record: stopped
	// means the SPA no longer loads its extension. If it has a backend, the
	// backend process and record move together.
	if !application.NeedsContainer() {
		if err := s.moveBackend(ctx, application, inst, target); err != nil {
			_ = s.saveStatus(ctx, &inst, StatusError, err.Error())
			return View{}, Instance{}, "", err
		}
		if err := s.saveStatus(ctx, &inst, target, ""); err != nil {
			return View{}, Instance{}, "", err
		}
		return s.view(inst), inst, previousStatus, nil
	}
	if s.installer == nil {
		return View{}, Instance{}, "", ErrUnavailable
	}
	if inst.Scope == ScopeProject && s.projects != nil && target == StatusRunning {
		if err := s.projects.EnsureRunning(ctx, inst.ProjectID); err != nil {
			return View{}, Instance{}, "", err
		}
	}
	upgrading := target == StatusRunning && needsUpgrade(inst, application)
	if upgrading {
		if err := reconcileInstanceEnv(application, &inst); err != nil {
			_ = s.saveStatus(ctx, &inst, StatusError, err.Error())
			return View{}, Instance{}, "", err
		}
	}
	if err := s.moveContainer(ctx, InstallSpec{Application: application, Instance: inst}, target); err != nil {
		_ = s.saveStatus(ctx, &inst, StatusError, err.Error())
		return View{}, Instance{}, "", err
	}
	// Only record the new version once the install that delivered it has
	// actually run, so a failed start leaves the instance asking for the
	// upgrade again rather than claiming to have it.
	if upgrading {
		inst.ApplicationVersion = application.Version
		inst.ContainerBuildVersion = application.containerBuildVersion()
	}
	if err := s.moveBackend(ctx, application, inst, target); err != nil {
		_ = s.saveStatus(ctx, &inst, StatusError, err.Error())
		return View{}, Instance{}, "", err
	}
	if err := s.saveStatus(ctx, &inst, target, ""); err != nil {
		return View{}, Instance{}, "", err
	}
	return s.view(inst), inst, previousStatus, nil
}

// moveContainer applies the requested lifecycle state to the container half
// of an application. The transition workflow deliberately runs this before moving a
// backend and recording the final status.
//
// Starting an instance whose application has moved on installs rather than starts.
// An upload upgrades the copies that are running and leaves stopped ones
// alone — bringing an app back up is not something an upload should decide —
// so this is where a stopped copy catches up, at the moment its owner asks for
// it. It is also how a built-in application upgraded by a Remote release reaches an
// app that was down when the release landed.
func (s *Service) moveContainer(ctx context.Context, spec InstallSpec, target InstanceStatus) error {
	if target != StatusRunning {
		return s.installer.Stop(ctx, spec)
	}
	if needsUpgrade(spec.Instance, spec.Application) {
		return s.installer.Install(ctx, spec)
	}
	return s.installer.Start(ctx, spec)
}
