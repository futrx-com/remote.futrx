package applications

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// Package is one uploaded application package: a ZIP holding exactly what an
// applications/<id>/ directory holds — application.json, an optional install.sh, an
// optional ui/, an optional backend/. Uploading one adds a catalog entry that
// installs, runs and renders like any entry the server was built with.
//
// A package is stored outside the binary, in the server's state directory, so
// updating Remote replaces the program and leaves uploaded applications, their
// installed instances and their settings exactly where they were.
//
// Every field below is part of that stored record: this struct is what the
// metadata file on disk holds, which is why what the API reports is a separate
// type. Changing a tag here migrates stored metadata, and adding a field to
// PackageView cannot.
type Package struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
	// Scopes are the scopes the packaged application declares. Uploading a package
	// adds it to a server-wide catalog, which is not the same as making it
	// installable everywhere: a project-only app is listed for every admin and
	// installable only inside a project. Carrying the scopes here is what lets
	// the management list say so instead of implying otherwise.
	Scopes []Scope `json:"scopes,omitempty"`
	// Filename is the name of the uploaded archive, kept for recognition only.
	Filename string `json:"filename,omitempty"`
	// Size and SHA256 describe the uploaded archive, so an operator can tell
	// two uploads of the same id apart.
	Size       int64  `json:"size"`
	SHA256     string `json:"sha256"`
	UploadedAt int64  `json:"uploadedAt"`
	UploadedBy string `json:"uploadedBy,omitempty"`
}

// PackageView is a stored package as the API reports it: the record on disk,
// plus what is only true of this server at this moment. None of the added
// fields is written to the metadata file, and none survives a restart — they
// are re-derived from the installed instances and from the catalog's last
// load. Package is embedded rather than copied field by field, so the JSON
// stays the single flat object the UI already reads.
type PackageView struct {
	Package
	// Installs are the copies of this package currently installed, in every
	// scope. Removing a package has to deal with them, so listing them is what
	// turns "uninstall this everywhere first" from a dead end into a decision
	// the caller can actually make.
	Installs []PackageInstall `json:"installs,omitempty"`
	// Upgraded reports the installed copies this upload re-provisioned because
	// its version differed from theirs. Empty when the version was unchanged,
	// when nothing is installed, or when the application reaches no container.
	Upgraded []UpgradeOutcome `json:"upgraded,omitempty"`
	// Error is set when a package that is still on disk no longer loads into
	// the catalog — an upload made against a different server version, say.
	// Reporting it beats dropping it silently: the files are still there, and
	// the operator is the one who can re-upload or remove them.
	Error string `json:"error,omitempty"`
}

// PackageInstall is one installed copy of a package, named well enough for a
// caller to recognise it before agreeing to remove it.
type PackageInstall struct {
	InstanceID string         `json:"instanceId"`
	Name       string         `json:"name"`
	Scope      Scope          `json:"scope"`
	ProjectID  string         `json:"projectId,omitempty"`
	Status     InstanceStatus `json:"status"`
}

// Packages lists the uploaded application packages this server stores.
func (s *Service) Packages(ctx context.Context) ([]PackageView, error) {
	if s.packages == nil {
		return nil, ErrPackagesUnavailable
	}
	list := s.packages.Packages()
	if list == nil {
		return []PackageView{}, nil
	}
	installs, err := s.installsByApplication(ctx)
	if err != nil {
		return nil, err
	}
	for i := range list {
		// A superseded package has no copies of its own: the instances under
		// its id are running the built-in application that replaced it. Listing them
		// here would offer to tear down working applications as the price of
		// deleting files nothing reads.
		if s.supersededByBuiltin(list[i].ID) {
			continue
		}
		list[i].Installs = installs[list[i].ID]
	}
	return list, nil
}

// supersededByBuiltin reports whether this package's id is served by an application
// compiled into the binary. That can only be true of a package the catalog
// refused to load, because an upload is checked against the built-in ids
// before it is written — so it means the stored files are shadowed, and
// whatever is installed under the id belongs to the built-in application now.
func (s *Service) supersededByBuiltin(id string) bool {
	application, ok := s.registry.Get(id)
	return ok && application.Source == SourceBuiltin
}

// installsByApplication groups every installed instance by the application it came from.
func (s *Service) installsByApplication(ctx context.Context) (map[string][]PackageInstall, error) {
	instances, err := s.store.ListAll(ctx)
	if err != nil {
		return nil, err
	}
	byApplication := map[string][]PackageInstall{}
	for _, inst := range instances {
		byApplication[inst.ApplicationID] = append(byApplication[inst.ApplicationID], PackageInstall{
			InstanceID: inst.ID,
			Name:       inst.Name,
			Scope:      inst.Scope,
			ProjectID:  inst.ProjectID,
			Status:     inst.Status,
		})
	}
	return byApplication, nil
}

// UploadPackage installs an uploaded archive into the catalog, replacing an
// earlier upload of the same id.
//
// Replacing a package refreshes what the catalog serves from it — its
// metadata, its browser extension, and the source its backend is compiled from
// — for every installed copy, because none of that lives in a container.
//
// The container side is different: re-running an install script provisions
// software inside a container someone is using, so it happens only when the
// package's `version` differs from the one an instance recorded. That is the
// signal an author controls: bump the version and every installed copy is
// re-provisioned; keep it and a re-upload changes only what is free to change.
// The per-instance results come back on the returned package.
func (s *Service) UploadPackage(ctx context.Context, upload PackageUpload) (PackageView, error) {
	if s.packages == nil {
		return PackageView{}, ErrPackagesUnavailable
	}
	if len(upload.Data) == 0 {
		return PackageView{}, fmt.Errorf("%w: the archive is empty", ErrPackageInvalid)
	}
	stored, err := s.packages.AddPackage(upload)
	if err != nil {
		return PackageView{}, err
	}
	if stored.Replaced {
		// AddPackage has already atomically swapped the registry view. Retire
		// every old process before upgrading: a later call must use the new
		// source even while container reconciliation is still in progress.
		// Application-wide invalidation also covers launches not yet visible in
		// the instance store and remains correct when ListAll fails below.
		if s.backends != nil {
			s.backends.InvalidateApplication(stored.ID)
		}
	} else {
		s.publishApplicationAdded(ctx, stored.ID)
	}
	pkg := PackageView{Package: stored.Package}
	// Re-provision the container side of every instance the new version made
	// stale. A replacement already invalidated all of its backend processes, so
	// every later call comes back on the current source.
	instances, err := s.store.ListAll(ctx)
	if err != nil {
		if stored.Replaced {
			s.publishApplicationUpdated(ctx, stored.ID)
		}
		return pkg, nil
	}
	pkg.Upgraded = s.upgradeInstances(ctx, pkg.ID, instances)
	// Publish only after every stale running copy has either converged or
	// recorded its failure. The event may be routed back to a newly declared
	// subscriber; it must not lazily launch that backend against old container
	// state while an upgrade is still waiting for the same instance lock.
	if stored.Replaced {
		s.publishApplicationUpdated(ctx, stored.ID)
	}
	return pkg, nil
}

// RemovePackageRequest asks for a package to be deleted.
type RemovePackageRequest struct {
	ID string
	// UninstallInstalled uninstalls every installed copy first. Without it a
	// package that is in use is refused, because deleting it out from under a
	// running app would leave that app pointing at a catalog entry that no
	// longer exists.
	//
	// This is the caller's decision rather than the server's, because the two
	// answers destroy different things: refusing costs a round trip, and
	// uninstalling can delete a database's container along with its data.
	UninstallInstalled bool
}

// RemovePackage deletes an uploaded package.
//
// It returns the copies it uninstalled on the way, so the caller can report
// what actually happened rather than just that the package is gone.
func (s *Service) RemovePackage(ctx context.Context, req RemovePackageRequest) ([]PackageInstall, error) {
	if s.packages == nil {
		return nil, ErrPackagesUnavailable
	}
	installs, err := s.installsOf(ctx, req.ID)
	if err != nil {
		return nil, err
	}
	// Deleting a superseded package strands nothing: the copies under its id
	// are already being served by the built-in application that shadowed it, and
	// they go on being served after its files are gone. Uninstalling them —
	// even when asked — would destroy applications to tidy a directory.
	if s.supersededByBuiltin(req.ID) {
		installs = nil
	}
	if len(installs) > 0 && !req.UninstallInstalled {
		// Naming where it is installed is the difference between an error a
		// caller can act on and one that just says no: "uninstall it
		// everywhere" is useless advice without an "everywhere".
		return nil, fmt.Errorf("%w: %s", ErrPackageInUse, describeInstalls(installs))
	}
	for i := range installs {
		if err := s.uninstallForPackageRemoval(ctx, req.ID, installs[i]); err != nil {
			// The list is what a caller reports on a removal that happened, so
			// one that did not returns only the failure — which already names
			// the copy that stopped it and why.
			return nil, fmt.Errorf(
				"%w: could not uninstall %s: %s", ErrPackageInUse, describeInstall(installs[i]), err)
		}
	}
	if err := s.packages.RemovePackage(req.ID); err != nil {
		return installs, err
	}
	s.publishApplicationDeleted(ctx, req.ID)
	return installs, nil
}

// uninstallForPackageRemoval removes one copy on the way to deleting its
// package.
//
// An application the catalog can no longer load — a package broken by a server
// update, say — cannot be torn down the ordinary way, because the description
// of what to tear down is exactly what is missing. Dropping the record is then
// the only move that does not strand the operator with a package they can
// neither repair nor remove.
//
// The half that does not need the application is still cleaned up: a backend process
// is addressed by instance id alone, so it is stopped and its data deleted
// rather than left running as a child of the server that nothing points at any
// more. Only the container side — which needs the application to describe it — is
// left, and is the operator's to clean up in LXD.
func (s *Service) uninstallForPackageRemoval(
	ctx context.Context,
	applicationID string,
	install PackageInstall,
) error {
	err := s.Uninstall(ctx, install.InstanceID)
	switch {
	case err == nil, errors.Is(err, ErrNotFound):
		return nil
	case errors.Is(err, ErrUnknownApplication):
		return s.removeUnknownPackageInstance(ctx, applicationID, install.InstanceID)
	default:
		return err
	}
}

func (s *Service) removeUnknownPackageInstance(
	ctx context.Context,
	applicationID, instanceID string,
) error {
	unlock := s.instanceLocks.lock(instanceID)
	defer unlock()

	current, found, err := s.store.Get(ctx, instanceID)
	if err != nil || !found {
		return err
	}
	if current.ApplicationID != applicationID {
		return fmt.Errorf(
			"instance %s no longer belongs to application %s",
			instanceID,
			applicationID,
		)
	}
	if s.backends != nil {
		if err := s.backends.Remove(ctx, instanceID); err != nil {
			return err
		}
	}
	if err := s.store.Delete(ctx, instanceID); err != nil {
		return err
	}
	s.publishApplicationUninstalled(ctx, current)
	return nil
}

// installsOf lists the installed copies of one application.
func (s *Service) installsOf(ctx context.Context, applicationID string) ([]PackageInstall, error) {
	byApplication, err := s.installsByApplication(ctx)
	if err != nil {
		return nil, err
	}
	return byApplication[applicationID], nil
}

func describeInstalls(installs []PackageInstall) string {
	described := make([]string, 0, len(installs))
	for _, install := range installs {
		described = append(described, describeInstall(install))
	}
	return "still installed " + strings.Join(described, ", ")
}

func describeInstall(install PackageInstall) string {
	if install.Scope == ScopeProject {
		return fmt.Sprintf("in project %s", install.ProjectID)
	}
	return "globally"
}
