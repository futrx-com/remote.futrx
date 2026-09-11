package applications

import "context"

// Upgrading an installed app to a new version of its image.
//
// An instance records the `version` from the image.json it was installed from.
// When the catalog's version for that image no longer matches, the container
// side is stale: the install script that provisioned it belonged to a
// different release. Re-running that script is what makes it current, and the
// recorded version is what tells us it has to happen.
//
// The comparison is equality, not ordering. Versions are free text — "8.0",
// "16", "1.2.3-rc1" — so there is no ordering to read, and none is invented:
// a version that *differs* re-installs, whether that is forward or back. An
// author who changes nothing keeps the version and nothing is re-run.
//
// What is deliberately not re-run: nothing at all for a `ui` or `backend`
// image, because neither provisions anything into a container. Their new code
// is picked up by reloading the catalog and restarting the plugin, which
// happens on every package replacement regardless of version.

// UpgradeOutcome is what happened to one instance when its image's version
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

// upgradeInstances re-runs the install script for every instance of an image
// whose recorded version differs from the catalog's.
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
func (s *Service) upgradeInstances(ctx context.Context, imageID string) []UpgradeOutcome {
	img, ok := s.registry.Get(imageID)
	if !ok {
		return nil
	}
	instances, err := s.store.ListAll(ctx)
	if err != nil {
		return nil
	}
	var outcomes []UpgradeOutcome
	for _, inst := range instances {
		if inst.ImageID != imageID || inst.Status == StatusStopped {
			continue
		}
		if !needsUpgrade(inst, img) {
			continue
		}
		outcome := UpgradeOutcome{
			InstanceID: inst.ID,
			Name:       inst.Name,
			Scope:      inst.Scope,
			ProjectID:  inst.ProjectID,
			From:       inst.ImageVersion,
			To:         img.Version,
		}
		if err := s.reinstall(ctx, img, &inst); err != nil {
			outcome.Error = err.Error()
			_ = s.saveStatus(ctx, &inst, StatusError, err.Error())
		}
		outcomes = append(outcomes, outcome)
	}
	return outcomes
}

// needsUpgrade reports whether an instance's container side was provisioned by
// a different version of the image than the catalog now holds.
//
// Only kinds that reach a container can be stale. A ui or backend image
// installs nothing to re-install, so bumping its version is a catalog change
// and nothing more.
func needsUpgrade(inst Instance, img Image) bool {
	return img.Type.NeedsContainer() && inst.ImageVersion != img.Version
}

// reinstall re-runs an instance's install script against the current image and
// records the version it now holds. It reuses the instance's existing
// container, port and resolved env, so an upgrade changes the software without
// changing where the app lives or what clients already connect to.
func (s *Service) reinstall(ctx context.Context, img Image, inst *Instance) error {
	if s.installer == nil {
		return ErrUnavailable
	}
	if inst.Scope == ScopeProject && s.projects != nil {
		if err := s.projects.EnsureRunning(ctx, inst.ProjectID); err != nil {
			return err
		}
	}
	// The plugin is stopped first so the install script is not running
	// alongside a process holding the software it is replacing. It comes back
	// on the next call to it, compiled from the source the new package shipped.
	if err := s.stopBackend(ctx, img, *inst); err != nil {
		return err
	}
	if err := s.installer.Install(ctx, InstallSpec{Image: img, Instance: *inst}); err != nil {
		return err
	}
	inst.ImageVersion = img.Version
	return s.saveStatus(ctx, inst, StatusRunning, "")
}
