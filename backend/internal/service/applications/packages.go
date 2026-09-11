package applications

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// Package is one uploaded application package: a ZIP holding exactly what an
// images/<id>/ directory holds — image.json, an optional install.sh, an
// optional ui/, an optional plugin/. Uploading one adds a catalog entry that
// installs, runs and renders like any entry the server was built with.
//
// A package is stored outside the binary, in the server's state directory, so
// updating Remote replaces the program and leaves uploaded applications, their
// installed instances and their settings exactly where they were.
type Package struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
	Type    Kind   `json:"type,omitempty"`
	// Scopes are the scopes the packaged image declares. Uploading a package
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
	// Installs are the copies of this package currently installed, in every
	// scope. Removing a package has to deal with them, so listing them is what
	// turns "uninstall this everywhere first" from a dead end into a decision
	// the caller can actually make.
	Installs []PackageInstall `json:"installs,omitempty"`
	// Upgraded reports the installed copies this upload re-provisioned because
	// its version differed from theirs. Empty when the version was unchanged,
	// when nothing is installed, or when the image reaches no container.
	Upgraded []UpgradeOutcome `json:"upgraded,omitempty"`
	// Error is set when a package that is still on disk no longer loads into
	// the catalog — an upload made against a different server version, say.
	// Reporting it beats dropping it silently: the files are still there, and
	// the operator is the one who can re-upload or remove them.
	Error string `json:"error,omitempty"`
}

// Installed reports whether the package produced a usable catalog entry.
func (p Package) Installed() bool { return p.Error == "" }

// PackageInstall is one installed copy of a package, named well enough for a
// caller to recognise it before agreeing to remove it.
type PackageInstall struct {
	InstanceID string         `json:"instanceId"`
	Name       string         `json:"name"`
	Scope      Scope          `json:"scope"`
	ProjectID  string         `json:"projectId,omitempty"`
	Status     InstanceStatus `json:"status"`
}

// PackageUpload is one archive submitted for installation into the catalog.
type PackageUpload struct {
	// Filename is the client's name for the archive. It is recorded, never
	// used to derive the image id: the id comes from image.json.
	Filename string
	Data     []byte
	// Actor is the email of the administrator who uploaded it.
	Actor string
}

// PackageCatalog is the writable half of the catalog: the part backed by
// uploaded packages on disk rather than by images compiled into the binary.
// A server without it still serves its built-in catalog and reports uploads
// unavailable, which is what keeps the feature optional rather than required.
type PackageCatalog interface {
	// Packages lists every stored package, including ones that failed to load.
	Packages() []Package
	// InstallPackage validates an archive and adds or replaces the catalog
	// entry it carries. It returns ErrPackageInvalid for a malformed archive
	// and ErrPackageReserved for one whose id belongs to a built-in image.
	InstallPackage(upload PackageUpload) (Package, error)
	// RemovePackage deletes a stored package and its catalog entry.
	RemovePackage(id string) error
}

// Packages lists the uploaded application packages this server stores.
func (s *Service) Packages(ctx context.Context) ([]Package, error) {
	if s.packages == nil {
		return nil, ErrPackagesUnavailable
	}
	list := s.packages.Packages()
	if list == nil {
		return []Package{}, nil
	}
	installs, err := s.installsByImage(ctx)
	if err != nil {
		return nil, err
	}
	for i := range list {
		// A superseded package has no copies of its own: the instances under
		// its id are running the built-in image that replaced it. Listing them
		// here would offer to tear down working applications as the price of
		// deleting files nothing reads.
		if s.supersededByBuiltin(list[i].ID) {
			continue
		}
		list[i].Installs = installs[list[i].ID]
	}
	return list, nil
}

// supersededByBuiltin reports whether this package's id is served by an image
// compiled into the binary. That can only be true of a package the catalog
// refused to load, because an upload is checked against the built-in ids
// before it is written — so it means the stored files are shadowed, and
// whatever is installed under the id belongs to the built-in image now.
func (s *Service) supersededByBuiltin(id string) bool {
	img, ok := s.registry.Get(id)
	return ok && img.Source == SourceBuiltin
}

// installsByImage groups every installed instance by the image it came from.
func (s *Service) installsByImage(ctx context.Context) (map[string][]PackageInstall, error) {
	instances, err := s.store.ListAll(ctx)
	if err != nil {
		return nil, err
	}
	byImage := map[string][]PackageInstall{}
	for _, inst := range instances {
		byImage[inst.ImageID] = append(byImage[inst.ImageID], PackageInstall{
			InstanceID: inst.ID,
			Name:       inst.Name,
			Scope:      inst.Scope,
			ProjectID:  inst.ProjectID,
			Status:     inst.Status,
		})
	}
	return byImage, nil
}

// UploadPackage installs an uploaded archive into the catalog, replacing an
// earlier upload of the same id.
//
// Replacing a package refreshes what the catalog serves from it — its
// metadata, its browser extension, and the source its plugin is compiled from
// — for every installed copy, because none of that lives in a container.
//
// The container side is different: re-running an install script provisions
// software inside a container someone is using, so it happens only when the
// package's `version` differs from the one an instance recorded. That is the
// signal an author controls: bump the version and every installed copy is
// re-provisioned; keep it and a re-upload changes only what is free to change.
// The per-instance results come back on the returned package.
func (s *Service) UploadPackage(ctx context.Context, upload PackageUpload) (Package, error) {
	if s.packages == nil {
		return Package{}, ErrPackagesUnavailable
	}
	if len(upload.Data) == 0 {
		return Package{}, fmt.Errorf("%w: the archive is empty", ErrPackageInvalid)
	}
	pkg, err := s.packages.InstallPackage(upload)
	if err != nil {
		return Package{}, err
	}
	// Re-provision the container side of every instance the new version made
	// stale. This also stops each plugin it touches, so those come back on the
	// new source by itself.
	pkg.Upgraded = s.upgradeInstances(ctx, pkg.ID)

	// Every other instance of the image still holds a plugin process running
	// the binary compiled from the previous upload. Stopping it is what makes
	// the new code take effect: the next call rebuilds and starts fresh.
	s.stopBackendsForImage(ctx, pkg.ID, upgradedIDs(pkg.Upgraded))
	return pkg, nil
}

// upgradedIDs collects the instances upgradeInstances already handled, so the
// plugin sweep below does not stop the same process twice.
func upgradedIDs(outcomes []UpgradeOutcome) map[string]bool {
	handled := make(map[string]bool, len(outcomes))
	for _, outcome := range outcomes {
		handled[outcome.InstanceID] = true
	}
	return handled
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
	// are already being served by the built-in image that shadowed it, and
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
		if err := s.uninstallForPackageRemoval(ctx, installs[i]); err != nil {
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
	return installs, nil
}

// uninstallForPackageRemoval removes one copy on the way to deleting its
// package.
//
// An image the catalog can no longer load — a package broken by a server
// update, say — cannot be torn down the ordinary way, because the description
// of what to tear down is exactly what is missing. Dropping the record is then
// the only move that does not strand the operator with a package they can
// neither repair nor remove.
//
// The half that does not need the image is still cleaned up: a plugin process
// is addressed by instance id alone, so it is stopped and its data deleted
// rather than left running as a child of the server that nothing points at any
// more. Only the container side — which needs the image to describe it — is
// left, and is the operator's to clean up in LXD.
func (s *Service) uninstallForPackageRemoval(ctx context.Context, install PackageInstall) error {
	err := s.Uninstall(ctx, install.InstanceID)
	switch {
	case err == nil, errors.Is(err, ErrNotFound):
		return nil
	case errors.Is(err, ErrUnknownImage):
		if s.backends != nil {
			if err := s.backends.Remove(ctx, install.InstanceID); err != nil {
				return err
			}
		}
		return s.store.Delete(ctx, install.InstanceID)
	default:
		return err
	}
}

// installsOf lists the installed copies of one image.
func (s *Service) installsOf(ctx context.Context, imageID string) ([]PackageInstall, error) {
	byImage, err := s.installsByImage(ctx)
	if err != nil {
		return nil, err
	}
	return byImage[imageID], nil
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

// stopBackendsForImage terminates the plugin process of every instance created
// from the image. Each one restarts on its next call, so this is a refresh
// rather than a shutdown; a failure to stop one is not worth failing an upload
// that already succeeded, so it is left to the caller's next request to retry.
func (s *Service) stopBackendsForImage(ctx context.Context, imageID string, skip map[string]bool) {
	if s.backends == nil {
		return
	}
	instances, err := s.store.ListAll(ctx)
	if err != nil {
		return
	}
	for _, inst := range instances {
		if inst.ImageID == imageID && !skip[inst.ID] {
			_ = s.backends.Stop(ctx, inst.ID)
		}
	}
}
