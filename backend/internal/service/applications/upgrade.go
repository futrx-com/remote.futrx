package applications

import (
	"context"
	"errors"
	"fmt"
)

// RestoreProject reprovisions running applications after the project's root
// filesystem is replaced. Their instance records live outside the container;
// without this step the UI would claim they are running while their packages
// and systemd units no longer exist. Stopped copies stay stopped.
func (s *Service) RestoreProject(ctx context.Context, projectID string) error {
	instances, err := s.store.ListProject(ctx, projectID)
	if err != nil {
		return err
	}
	var failures []error
	for _, snapshot := range instances {
		if snapshot.Status != StatusRunning {
			continue
		}
		unlock, acquired := s.instanceLocks.tryLock(snapshot.ID)
		if !acquired {
			continue
		}
		inst, application, err := s.load(ctx, snapshot.ID)
		if err == nil && inst.Status == StatusRunning && application.NeedsContainer() {
			if s.installer == nil {
				err = ErrUnavailable
			} else if err = reconcileInstanceEnv(application, &inst); err == nil {
				err = s.stopBackend(ctx, application, inst)
			}
			if err == nil {
				err = s.installer.Install(ctx, InstallSpec{Application: application, Instance: inst})
			}
			if err == nil {
				inst.ApplicationVersion = application.Version
				inst.ContainerBuildVersion = application.containerBuildVersion()
				err = s.saveStatus(ctx, &inst, StatusRunning, "")
			}
			if err != nil {
				_ = s.saveStatus(ctx, &inst, StatusError, err.Error())
			}
		}
		unlock()
		if err != nil {
			failures = append(failures, fmt.Errorf("restore application %s: %w", snapshot.ApplicationID, err))
		}
	}
	return errors.Join(failures...)
}

// Upgrading an installed app to a new version of its application.
//
// An instance records the `version` from the application.json it was installed
// from and, when present, the build identity derived from backend/container/.
// When either no longer matches the catalog, the container side is stale.
// Re-running the install script is what makes it current.
//
// The comparison is equality, not ordering. Versions are free text — "8.0",
// "16", "1.2.3-rc1" — so there is no ordering to read, and none is invented:
// a version that *differs* re-installs, whether that is forward or back. An
// author who changes neither the manifest version nor container source keeps
// the same identities and nothing is re-run.
//
// What is deliberately not re-run: nothing at all for a UI-only or host-
// backend-only application, because neither provisions anything into a
// container. Their new code is picked up by reloading the catalog and
// restarting the backend, which happens on every package replacement regardless
// of version.

// UpgradeOutcome is what happened to one instance when its application's version
// moved. It is reported rather than logged: an upgrade re-runs an install
// script inside a container someone is using, so its result is something the
// administrator who triggered it has to be able to read.
type UpgradeOutcome struct {
	InstanceID string `json:"instanceId"`
	Name       string `json:"name"`
	Scope      Scope  `json:"scope"`
	ProjectID  string `json:"projectId,omitempty"`
	// From is the version the instance had recorded; empty for an instance
	// installed before versions were tracked.
	From string `json:"fromVersion,omitempty"`
	To   string `json:"toVersion"`
	// Error is set when the re-install failed. The instance is left in the
	// error state with its old version recorded, so the next upload — or a
	// manual start — tries again rather than skipping it as already current.
	Error string `json:"error,omitempty"`
}

// upgradeInstances re-runs the install script for every instance whose recorded
// application version or container build identity differs from the catalog's.
//
// Stopped instances are left alone: re-running an install script also brings
// the app up, and resurrecting an app an operator deliberately stopped is not
// something an upload should do. They are upgraded when they are next started
// — see Start.
//
// A failure upgrades no further than that one instance: the others are still
// attempted, and each carries its own result. The upload that triggered this
// already succeeded, and reporting a per-instance failure is more useful than
// pretending the package was never stored.
func (s *Service) upgradeInstances(ctx context.Context, applicationID string, instances []Instance) []UpgradeOutcome {
	if _, ok := s.registry.Get(applicationID); !ok {
		return nil
	}
	var outcomes []UpgradeOutcome
	for _, snapshot := range instances {
		if snapshot.ApplicationID != applicationID {
			continue
		}
		if outcome, upgraded := s.upgradeInstance(ctx, applicationID, snapshot.ID); upgraded {
			outcomes = append(outcomes, outcome)
		}
	}
	return outcomes
}

// upgradeInstance reloads and re-evaluates the candidate under its instance
// lock. UploadPackage supplies a ListAll snapshot, but a concurrent Stop or
// Uninstall may have committed since that snapshot was taken; stale data must
// never resurrect or recreate that copy.
func (s *Service) upgradeInstance(
	ctx context.Context,
	applicationID, instanceID string,
) (UpgradeOutcome, bool) {
	unlock := s.instanceLocks.lock(instanceID)
	defer unlock()

	inst, application, err := s.load(ctx, instanceID)
	if err != nil || inst.ApplicationID != applicationID || inst.Status == StatusStopped {
		return UpgradeOutcome{}, false
	}
	if !needsUpgrade(inst, application) {
		return UpgradeOutcome{}, false
	}
	outcome := UpgradeOutcome{
		InstanceID: inst.ID,
		Name:       inst.Name,
		Scope:      inst.Scope,
		ProjectID:  inst.ProjectID,
		From:       inst.ApplicationVersion,
		To:         application.Version,
	}
	if err := s.reinstall(ctx, application, &inst); err != nil {
		outcome.Error = err.Error()
		_ = s.saveStatus(ctx, &inst, StatusError, err.Error())
	}
	return outcome, true
}

// needsUpgrade reports whether an instance's container side was provisioned by
// a different application release or container build than the catalog now
// holds.
//
// Only applications that reach a container can be stale. A backend-only
// application installs nothing to re-install; its new code is picked up by
// restarting the backend.
func needsUpgrade(inst Instance, application Application) bool {
	return application.NeedsContainer() && (inst.ApplicationVersion != application.Version ||
		inst.ContainerBuildVersion != application.containerBuildVersion())
}

// reinstall re-runs an instance's install script against the current application and
// records the version it now holds. It reuses the instance's existing
// container, port and resolved env, so an upgrade changes the software without
// changing where the app lives or what clients already connect to.
func (s *Service) reinstall(ctx context.Context, application Application, inst *Instance) error {
	if s.installer == nil {
		return ErrUnavailable
	}
	if err := reconcileInstanceEnv(application, inst); err != nil {
		return err
	}
	if inst.Scope == ScopeProject && s.projects != nil {
		if err := s.projects.EnsureRunning(ctx, inst.ProjectID); err != nil {
			return err
		}
	}
	// The backend is stopped first so the install script is not running
	// alongside a process holding the software it is replacing. It comes back
	// on the next call to it, compiled from the source the new package shipped.
	if err := s.stopBackend(ctx, application, *inst); err != nil {
		return err
	}
	if err := s.installer.Install(ctx, InstallSpec{Application: application, Instance: *inst}); err != nil {
		return err
	}
	inst.ApplicationVersion = application.Version
	inst.ContainerBuildVersion = application.containerBuildVersion()
	return s.saveStatus(ctx, inst, StatusRunning, "")
}

// reconcileInstanceEnv projects persisted inputs onto the current manifest.
// It preserves values that still exist, applies defaults/generators for new
// declarations, and drops removed keys so an upgrade cannot retain obsolete
// credentials or expose them after their secret declaration disappears.
func reconcileInstanceEnv(application Application, inst *Instance) error {
	env, err := resolveEnv(application, inst.Env)
	if err != nil {
		return err
	}
	inst.Env = env
	return nil
}
