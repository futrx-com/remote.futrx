package applications

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/futrx-com/remote.futrx.com/pkg/applications"
)

// Errors specific to the backend half of an application.
var (
	// ErrNoBackend is returned for an instance whose application ships no backend.
	ErrNoBackend = errors.New("applications: application has no backend backend")
	// ErrBackendAccess is returned when the application restricts its backend to
	// administrators and the caller is not one.
	ErrBackendAccess = errors.New("applications: backend restricted to administrators")
	// ErrNotRunning is returned when a backend is asked for while its instance
	// is stopped: a stopped app's backend is off, exactly as its UI is.
	ErrNotRunning = errors.New("applications: application is not running")
	// ErrBackendContext is returned when core could not resolve a complete,
	// trusted context for a scoped backend call.
	ErrBackendContext = errors.New("applications: invalid backend context")
	// ErrBackendContextAccess is returned when an instance does not belong to
	// the project containing the chat through which it was addressed.
	ErrBackendContextAccess = errors.New("applications: backend unavailable in chat context")
)

// DescribeBackend starts the instance's backend if needed and returns what it says
// about itself, including the routes an extension may call.
func (s *Service) DescribeBackend(ctx context.Context, id string, caller applications.Caller) (BackendDescriptor, error) {
	return s.describeBackend(ctx, id, caller, nil)
}

// DescribeBackendForChat describes a backend only when the install may serve
// the already-authorized chat. Global installs serve every chat; a project
// install serves chats in that exact project.
func (s *Service) DescribeBackendForChat(
	ctx context.Context,
	id string,
	caller applications.Caller,
	chat applications.ChatContext,
) (BackendDescriptor, error) {
	return s.describeBackend(ctx, id, caller, &chat)
}

func (s *Service) describeBackend(
	ctx context.Context,
	id string,
	caller applications.Caller,
	chat *applications.ChatContext,
) (BackendDescriptor, error) {
	unlock := s.instanceLocks.rlock(id)
	defer unlock()

	instance, application, err := s.backendInstance(ctx, id, caller)
	if err != nil {
		return BackendDescriptor{}, err
	}
	if err := validateBackendChat(instance, chat); err != nil {
		return BackendDescriptor{}, err
	}
	ctx, cancel := s.backendDeadline(ctx, application)
	defer cancel()

	descriptor, err := s.backends.Ensure(ctx, instance)
	if err != nil {
		return BackendDescriptor{}, err
	}
	return BackendDescriptor{
		InstanceID:    instance.ID,
		ApplicationID: instance.ApplicationID,
		Descriptor:    descriptor,
		Access:        application.Audience(),
		TimeoutMS:     application.Timeout(),
	}, nil
}

// CallBackend forwards one request to an instance's backend. The caller is
// resolved by the transport and stamped here rather than read from the
// request, so a backend can trust Request.Caller no matter what the browser
// sent.
func (s *Service) CallBackend(
	ctx context.Context,
	id string,
	request applications.Request,
	caller applications.Caller,
) (applications.Response, error) {
	return s.callBackend(ctx, id, request, caller, nil)
}

// CallBackendForChat forwards a request with chat context that core has
// resolved and authorized. The supplied request cannot override either the
// caller or context stamped here.
func (s *Service) CallBackendForChat(
	ctx context.Context,
	id string,
	request applications.Request,
	caller applications.Caller,
	chat applications.ChatContext,
) (applications.Response, error) {
	return s.callBackend(ctx, id, request, caller, &chat)
}

func (s *Service) callBackend(
	ctx context.Context,
	id string,
	request applications.Request,
	caller applications.Caller,
	chat *applications.ChatContext,
) (applications.Response, error) {
	unlock := s.instanceLocks.rlock(id)
	defer unlock()

	instance, application, err := s.backendInstance(ctx, id, caller)
	if err != nil {
		return applications.Response{}, err
	}
	if err := validateBackendChat(instance, chat); err != nil {
		return applications.Response{}, err
	}
	request.Caller = caller
	request.Context = applications.RequestContext{}
	if chat != nil {
		trusted := *chat
		request.Context.Chat = &trusted
	}

	ctx, cancel := s.backendDeadline(ctx, application)
	defer cancel()

	response, err := s.backends.Call(ctx, instance, request)
	if err != nil {
		return applications.Response{}, err
	}
	if response.Status == 0 {
		response.Status = 200
	}
	return response, nil
}

func validateBackendChat(instance applications.Instance, chat *applications.ChatContext) error {
	if chat == nil {
		return nil
	}
	if chat.ID == "" || chat.WorkspaceRoot == "" {
		return ErrBackendContext
	}
	if instance.Scope == string(ScopeGlobal) {
		return nil
	}
	if instance.Scope != string(ScopeProject) || chat.ProjectID == "" || instance.ProjectID != chat.ProjectID {
		return ErrBackendContextAccess
	}
	return nil
}

// backendInstance resolves an instance to a runnable backend, enforcing every
// precondition a call has: the application ships one, the app is running, and the
// caller is allowed to reach it.
func (s *Service) backendInstance(
	ctx context.Context,
	id string,
	caller applications.Caller,
) (applications.Instance, ApplicationBackend, error) {
	if s.backends == nil {
		return applications.Instance{}, ApplicationBackend{}, ErrUnavailable
	}
	instance, application, err := s.load(ctx, id)
	if err != nil {
		return applications.Instance{}, ApplicationBackend{}, err
	}
	if application.Backend == nil {
		return applications.Instance{}, ApplicationBackend{}, fmt.Errorf("%w: %s", ErrNoBackend, application.ID)
	}
	if instance.Status != StatusRunning {
		return applications.Instance{}, ApplicationBackend{}, fmt.Errorf("%w: %s", ErrNotRunning, instance.ID)
	}
	if application.Backend.Audience() == BackendAccessAdmin && !caller.IsAdmin {
		return applications.Instance{}, ApplicationBackend{}, fmt.Errorf("%w: %s", ErrBackendAccess, application.ID)
	}
	return backendInstanceDetails(application, instance), *application.Backend, nil
}

// backendDeadline bounds one backend call. A backend is a separate process the
// request goroutine waits on; without this, a backend that never answers holds
// the connection open forever.
func (s *Service) backendDeadline(ctx context.Context, application ApplicationBackend) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, time.Duration(application.Timeout())*time.Millisecond)
}

// ---- lifecycle -------------------------------------------------------------

// startBackend brings up an instance's backend, if it has one. An application with no
// backend, or a server with no backend host, is a no-op rather than an error, so
// the ordinary install path does not have to ask first.
func (s *Service) startBackend(ctx context.Context, application Application, instance Instance) error {
	if application.Backend == nil || s.backends == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, backendStartTimeout)
	defer cancel()
	if _, err := s.backends.Ensure(ctx, backendInstanceDetails(application, instance)); err != nil {
		return fmt.Errorf("start backend: %w", err)
	}
	return nil
}

// stopBackend terminates an instance's backend process, keeping its data.
func (s *Service) stopBackend(ctx context.Context, application Application, instance Instance) error {
	if application.Backend == nil || s.backends == nil {
		return nil
	}
	return s.backends.Stop(ctx, instance.ID)
}

// removeBackend terminates an instance's backend and discards its data
// directory. Uninstalling is the only thing that deletes a backend's state.
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

func backendInstanceDetails(application Application, instance Instance) applications.Instance {
	return applications.Instance{
		ID:                 instance.ID,
		ApplicationID:      instance.ApplicationID,
		ApplicationName:    application.Name,
		ApplicationVersion: application.Version,
		Publishers:         application.Publishers,
		Subscriptions:      application.Subscriptions,
		Service:            application.ServiceName(),
		Scope:              string(instance.Scope),
		ProjectID:          instance.ProjectID,
		ContainerName:      instance.ContainerName,
		InternalPort:       instance.InternalPort,
		ExternalPort:       instance.ExternalPort,
		Env:                instance.Env,
	}
}

// moveBackend brings an instance's backend in line with the status it is
// transitioning to.
func (s *Service) moveBackend(ctx context.Context, application Application, instance Instance, target InstanceStatus) error {
	if target == StatusRunning {
		return s.startBackend(ctx, application, instance)
	}
	return s.stopBackend(ctx, application, instance)
}
