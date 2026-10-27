package applications

import (
	"context"
	"errors"
	"sync"
)

var ErrInstallInProgress = errors.New("applications: an installation is already running")

// InstallRun is the latest attempt for an application in a particular scope.
// It never contains install inputs or resolved credentials.
type InstallRun struct {
	ID            string          `json:"id"`
	ApplicationID string          `json:"applicationId"`
	Scope         Scope           `json:"scope"`
	ProjectID     string          `json:"projectId,omitempty"`
	State         string          `json:"state"`
	Error         string          `json:"error,omitempty"`
	Log           string          `json:"log,omitempty"`
	Progress      InstallProgress `json:"progress"`
}

// WithInstallRuns binds observed installs to server shutdown.
// The legacy synchronous Install API remains available to application tooling.
func WithInstallRuns(ctx context.Context) Option {
	return func(s *Service) {
		s.installRuns.ctx = ctx
	}
}

type installRunKey struct {
	scope        Scope
	project, app string
}

type installRuns struct {
	mu   sync.Mutex
	ctx  context.Context
	runs map[installRunKey]*observedInstall
}

type observedInstall struct {
	mu  sync.Mutex
	run InstallRun
	log installLog
}

func (s *Service) StartInstall(ctx context.Context, req InstallRequest) (InstallRun, error) {
	application, ok := s.registry.Get(req.ApplicationID)
	if !ok {
		return InstallRun{}, ErrUnknownApplication
	}
	if !req.Scope.Valid() || !application.SupportsScope(req.Scope) {
		return InstallRun{}, ErrScope
	}
	if req.Scope == ScopeProject && req.ProjectID == "" {
		return InstallRun{}, ErrProjectneeded
	}
	if req.ExternalPort < 0 || req.ExternalPort > 65535 {
		return InstallRun{}, ErrPortRange
	}
	if _, err := resolveEnv(application, req.Env); err != nil {
		return InstallRun{}, err
	}
	if err := ctx.Err(); err != nil {
		return InstallRun{}, err
	}
	// Do not let a caller mutate values after the worker takes ownership.
	env := make(map[string]string, len(req.Env))
	for key, value := range req.Env {
		env[key] = value
	}
	req.Env = env
	key := installRunKey{req.Scope, req.ProjectID, req.ApplicationID}
	m := &s.installRuns
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.runs == nil {
		m.runs = make(map[installRunKey]*observedInstall)
	}
	if previous := m.runs[key]; previous != nil {
		if snapshot := previous.snapshot(); snapshot.State == "running" {
			return snapshot, ErrInstallInProgress
		}
	} else {
		instances, err := s.store.ListAll(ctx)
		if err != nil {
			return InstallRun{}, err
		}
		if err := s.recoverInterruptedInstall(ctx, instances, req.Scope, req.ProjectID, req.ApplicationID); err != nil {
			return InstallRun{}, err
		}
	}
	if !(req.Scope == ScopeGlobal && application.GloballyInstalledInsideContainers) {
		existing, found, err := s.instanceOfApplication(ctx, req.Scope, req.ProjectID, req.ApplicationID)
		if err != nil {
			return InstallRun{}, err
		}
		if found && existing.Status == StatusInstalling {
			return InstallRun{}, ErrInstallInProgress
		}
		if found && existing.Status != StatusError {
			return InstallRun{}, ErrAlreadyInstalled
		}
	}
	workerCtx := m.ctx
	if workerCtx == nil {
		workerCtx = context.Background()
	}
	if err := workerCtx.Err(); err != nil {
		return InstallRun{}, err
	}
	run := &observedInstall{
		run: InstallRun{ID: newInstanceID(), ApplicationID: req.ApplicationID, Scope: req.Scope,
			ProjectID: req.ProjectID, State: "running",
			Progress: InstallProgress{Phase: "preparing", Message: "Preparing installation"}},
	}
	run.log.protect(application, req.Env)
	m.runs[key] = run
	initial := run.snapshot()
	go func() {
		_, err := s.Install(context.WithValue(workerCtx, installObserverKey{}, run), req)
		run.finish(err)
	}()
	return initial, nil
}

// Installations returns live attempts. After a restart, existing instance
// records make interrupted work retryable; the old live log is discarded.
func (s *Service) Installations(ctx context.Context, scope Scope, project string) ([]InstallRun, error) {
	m := &s.installRuns
	m.mu.Lock()
	defer m.mu.Unlock()
	instances, err := s.store.ListAll(ctx)
	if err != nil {
		return nil, err
	}
	runs := []InstallRun{}
	for _, application := range s.registry.List() {
		if run := m.runs[installRunKey{scope, project, application.ID}]; run != nil {
			runs = append(runs, run.snapshot())
		} else if err := s.recoverInterruptedInstall(ctx, instances, scope, project, application.ID); err != nil {
			return nil, err
		}
	}
	return runs, nil
}

func (s *Service) recoverInterruptedInstall(ctx context.Context, instances []Instance, scope Scope, project, app string) error {
	application, _ := s.registry.Get(app)
	for _, inst := range instances {
		matches := inst.Scope == scope && inst.ProjectID == project
		if scope == ScopeGlobal && application.GloballyInstalledInsideContainers {
			matches = inst.Scope == ScopeProject
		}
		if matches && inst.ApplicationID == app && inst.Status == StatusInstalling {
			unlock, acquired := s.instanceLocks.tryLock(inst.ID)
			if !acquired {
				continue
			}
			// An active synchronous install also holds this lock. Re-read after
			// acquiring it in case that install completed during the list read.
			current, found, err := s.store.Get(ctx, inst.ID)
			if err == nil && found && current.Status == StatusInstalling {
				err = s.saveStatus(ctx, &current, StatusError, "Installation interrupted by a server restart. Retry to try again.")
			}
			unlock()
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *observedInstall) snapshot() InstallRun {
	r.mu.Lock()
	defer r.mu.Unlock()
	run := r.run
	run.Log = r.log.snapshot()
	return run
}

func (r *observedInstall) progress(progress InstallProgress) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.run.State != "running" {
		return
	}
	if progress.Total == 0 {
		progress.CurrentItem, progress.Completed, progress.Total = r.run.Progress.CurrentItem, r.run.Progress.Completed, r.run.Progress.Total
	}
	r.run.Progress = progress
	r.log.finish()
	r.log.write("\n" + progress.Message + "\n")
}

func (r *observedInstall) protect(application Application, env map[string]string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.log.protect(application, env)
}

func (r *observedInstall) redact(text string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.log.redact(text)
}

func (r *observedInstall) Write(data []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.run.State != "running" {
		return len(data), nil
	}
	r.log.write(string(data))
	return len(data), nil
}

func (r *observedInstall) finish(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.run.State = "succeeded"
	r.log.finish()
	if err != nil {
		r.run.State = "failed"
		r.run.Error = r.log.redact(err.Error())
		r.log.write("\nInstallation stopped: " + err.Error() + "\n")
	} else {
		r.run.Progress.Phase, r.run.Progress.Message = "complete", "Application installed"
		r.log.write("\nApplication installed\n")
	}
	r.log.finish()
}
