package applications

import (
	"context"
	"log"
)

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

// UpgradeOutdated reinstalls running copies whose container side is older than
// the catalog. A Remote release can change an application's container program
// without anyone starting it; without this, a backend compiled from the new
// source would keep talking to the old program until that copy happened to be
// restarted. Copies in project containers that are not running are left to
// UpgradeProject, which runs when their container next starts.
func (s *Service) UpgradeOutdated(ctx context.Context) []UpgradeOutcome {
	instances, err := s.store.ListAll(ctx)
	if err != nil {
		log.Printf("applications: list instances to upgrade: %v", err)
		return nil
	}
	running, _ := s.projects.(RunningProjects)
	var outcomes []UpgradeOutcome
	for _, snapshot := range instances {
		if ctx.Err() != nil {
			break
		}
		if snapshot.Status == StatusStopped {
			continue
		}
		if snapshot.Scope == ScopeProject {
			if running == nil {
				continue
			}
			if up, err := running.Running(ctx, snapshot.ProjectID); err != nil || !up {
				continue
			}
		}
		if outcome, upgraded := s.upgradeInstance(ctx, snapshot.ApplicationID, snapshot.ID); upgraded {
			logUpgrade(outcome)
			outcomes = append(outcomes, outcome)
		}
	}
	return outcomes
}

// UpgradeProject reinstalls the outdated running copies in one project. The
// project service calls it after starting a project container that was down,
// which is where a copy skipped by UpgradeOutdated catches up.
func (s *Service) UpgradeProject(ctx context.Context, projectID string) []UpgradeOutcome {
	instances, err := s.store.ListProject(ctx, projectID)
	if err != nil {
		log.Printf("applications: list project %s instances to upgrade: %v", projectID, err)
		return nil
	}
	var outcomes []UpgradeOutcome
	for _, snapshot := range instances {
		if ctx.Err() != nil {
			break
		}
		if outcome, upgraded := s.upgradeInstance(ctx, snapshot.ApplicationID, snapshot.ID); upgraded {
			logUpgrade(outcome)
			outcomes = append(outcomes, outcome)
		}
	}
	return outcomes
}

// logUpgrade records an upgrade nobody asked for. An upload reports its
// outcomes to the administrator; these happen on their own, so the log is
// where they are read.
func logUpgrade(outcome UpgradeOutcome) {
	if outcome.Error != "" {
		log.Printf("applications: upgrade %s (%s) from %q to %q failed: %s",
			outcome.Name, outcome.InstanceID, outcome.From, outcome.To, outcome.Error)
		return
	}
	log.Printf("applications: upgraded %s (%s) from %q to %q", outcome.Name, outcome.InstanceID, outcome.From, outcome.To)
}
