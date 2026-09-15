package applications

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/futrx-com/remote.futrx.com/pkg/appplugin"
)

// Errors specific to the backend half of an application.
var (
	// ErrNoBackend is returned for an instance whose application ships no plugin.
	ErrNoBackend = errors.New("applications: application has no backend plugin")
	// ErrBackendAccess is returned when the application restricts its plugin to
	// administrators and the caller is not one.
	ErrBackendAccess = errors.New("applications: backend restricted to administrators")
	// ErrNotRunning is returned when a plugin is asked for while its instance
	// is stopped: a stopped app's backend is off, exactly as its UI is.
	ErrNotRunning = errors.New("applications: application is not running")
)

// DescribeBackend starts the instance's plugin if needed and returns what it says
// about itself, including the routes an extension may call.
func (s *Service) DescribeBackend(ctx context.Context, id string, caller appplugin.Caller) (BackendDescriptor, error) {
	spec, application, err := s.backendSpec(ctx, id, caller)
	if err != nil {
		return BackendDescriptor{}, err
	}
	ctx, cancel := s.backendDeadline(ctx, application)
	defer cancel()

	descriptor, err := s.backends.Ensure(ctx, spec)
	if err != nil {
		return BackendDescriptor{}, err
	}
	return BackendDescriptor{
		InstanceID:    spec.Instance.ID,
		ApplicationID: spec.ApplicationID,
		Descriptor:    descriptor,
		Access:        application.Audience(),
		TimeoutMS:     application.Timeout(),
	}, nil
}

// CallBackend forwards one request to an instance's plugin. The caller is
// resolved by the transport and stamped here rather than read from the
// request, so a plugin can trust Request.Caller no matter what the browser
// sent.
func (s *Service) CallBackend(
	ctx context.Context,
	id string,
	request appplugin.Request,
	caller appplugin.Caller,
) (appplugin.Response, error) {
	spec, application, err := s.backendSpec(ctx, id, caller)
	if err != nil {
		return appplugin.Response{}, err
	}
	request.Caller = caller

	ctx, cancel := s.backendDeadline(ctx, application)
	defer cancel()

	response, err := s.backends.Call(ctx, spec, request)
	if err != nil {
		return appplugin.Response{}, err
	}
	if response.Status == 0 {
		response.Status = 200
	}
	return response, nil
}

// backendSpec resolves an instance to a runnable plugin, enforcing every
// precondition a call has: the application ships one, the app is running, and the
// caller is allowed to reach it.
func (s *Service) backendSpec(
	ctx context.Context,
	id string,
	caller appplugin.Caller,
) (BackendSpec, ApplicationBackend, error) {
	if s.backends == nil {
		return BackendSpec{}, ApplicationBackend{}, ErrUnavailable
	}
	instance, application, err := s.load(ctx, id)
	if err != nil {
		return BackendSpec{}, ApplicationBackend{}, err
	}
	if application.Backend == nil {
		return BackendSpec{}, ApplicationBackend{}, fmt.Errorf("%w: %s", ErrNoBackend, application.ID)
	}
	if instance.Status != StatusRunning {
		return BackendSpec{}, ApplicationBackend{}, fmt.Errorf("%w: %s", ErrNotRunning, instance.ID)
	}
	if application.Backend.Audience() == BackendAccessAdmin && !caller.IsAdmin {
		return BackendSpec{}, ApplicationBackend{}, fmt.Errorf("%w: %s", ErrBackendAccess, application.ID)
	}
	return newBackendSpec(application, instance), *application.Backend, nil
}

// backendDeadline bounds one plugin call. A plugin is a separate process the
// request goroutine waits on; without this, a plugin that never answers holds
// the connection open forever.
func (s *Service) backendDeadline(ctx context.Context, application ApplicationBackend) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, time.Duration(application.Timeout())*time.Millisecond)
}

// ---- lifecycle -------------------------------------------------------------

// startBackend brings up an instance's plugin, if it has one. An application with no
// plugin, or a server with no plugin host, is a no-op rather than an error, so
// the ordinary install path does not have to ask first.
func (s *Service) startBackend(ctx context.Context, application Application, instance Instance) error {
	if application.Backend == nil || s.backends == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, backendStartTimeout)
	defer cancel()
	if _, err := s.backends.Ensure(ctx, newBackendSpec(application, instance)); err != nil {
		return fmt.Errorf("start backend: %w", err)
	}
	return nil
}

// stopBackend terminates an instance's plugin process, keeping its data.
func (s *Service) stopBackend(ctx context.Context, application Application, instance Instance) error {
	if application.Backend == nil || s.backends == nil {
		return nil
	}
	return s.backends.Stop(ctx, instance.ID)
}

// removeBackend terminates an instance's plugin and discards its data
// directory. Uninstalling is the only thing that deletes a plugin's state.
func (s *Service) removeBackend(ctx context.Context, application Application, instance Instance) error {
	if application.Backend == nil || s.backends == nil {
		return nil
	}
	return s.backends.Remove(ctx, instance.ID)
}

// backendStartTimeout bounds the first launch, which may include compiling the
// application's source. It is generous because a cold build resolves modules; every
// later start hits the binary cache and takes milliseconds.
const backendStartTimeout = 5 * time.Minute

func newBackendSpec(application Application, instance Instance) BackendSpec {
	return BackendSpec{
		ApplicationID: application.ID,
		Instance: appplugin.Instance{
			ID:            instance.ID,
			ApplicationID: instance.ApplicationID,
			Scope:         string(instance.Scope),
			ProjectID:     instance.ProjectID,
			ContainerName: instance.ContainerName,
			InternalPort:  instance.InternalPort,
			ExternalPort:  instance.ExternalPort,
			Env:           instance.Env,
		},
	}
}

// moveBackend brings an instance's plugin in line with the status it is
// transitioning to.
func (s *Service) moveBackend(ctx context.Context, application Application, instance Instance, target InstanceStatus) error {
	if target == StatusRunning {
		return s.startBackend(ctx, application, instance)
	}
	return s.stopBackend(ctx, application, instance)
}
