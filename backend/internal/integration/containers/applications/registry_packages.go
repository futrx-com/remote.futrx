package applications

import (
	"errors"
	"fmt"

	svc "github.com/futrx-com/remote.futrx.com/internal/service/applications"
)

// The registry is what joins an uploaded package to the running catalog: the
// store owns the bytes on disk, and every write to it is followed by a reload
// so the installer, the application backend host and the HTTP handlers see the new entry
// without anything being rebuilt or restarted.

var _ svc.PackageCatalog = (*Registry)(nil)

// errPackageReserved states the one rule that keeps an upload from redefining
// a built-in application. Both the id check made before an archive is written
// and the one made while reloading the catalog answer with this, so an
// operator sees the same refusal wherever it is raised.
func errPackageReserved(id string) error {
	return fmt.Errorf("%w: %q", svc.ErrPackageReserved, id)
}

// errPackageSuperseded is what a stored package is told when the id it was
// uploaded under has since been built into the binary. Nothing was rejected —
// the built-in application is being served and these files are not — so the reason
// shown beside it in the package list says that, and says they can go.
func errPackageSuperseded(id string) error {
	return fmt.Errorf("%w: %q", svc.ErrPackageSuperseded, id)
}

// Packages lists the stored packages, annotating each with the reason it is
// not in the catalog when it failed to load.
func (r *Registry) Packages() []svc.PackageView {
	if r.packages == nil {
		return nil
	}
	stored := r.packages.list()

	r.mu.RLock()
	defer r.mu.RUnlock()

	listed := make([]svc.PackageView, 0, len(stored))
	for _, pkg := range stored {
		view := svc.PackageView{Package: pkg}
		if reason, failed := r.packageErrors[pkg.ID]; failed {
			view.Error = reason
			listed = append(listed, view)
			continue
		}
		// What the app *is* — its name, version and the scopes it may be
		// installed at — is whatever the catalog loaded from the files on disk,
		// not whatever was recorded when it was uploaded. Reading it back from
		// the catalog is what keeps the management list from describing a
		// package by a stale copy of its own manifest.
		if application, ok := r.view.byID[pkg.ID]; ok {
			view.Name = application.Name
			view.Version = application.Version
			view.Scopes = application.Scopes
		}
		listed = append(listed, view)
	}
	return listed
}

// AddPackage stores an uploaded archive and reloads the catalog.
func (r *Registry) AddPackage(upload svc.PackageUpload) (svc.PackageMutation, error) {
	if r.packages == nil {
		return svc.PackageMutation{}, svc.ErrPackagesUnavailable
	}
	mutation, err := r.packages.add(upload, r.reservePackageID)
	if err != nil {
		return svc.PackageMutation{}, err
	}
	if err := r.Reload(); err != nil {
		return svc.PackageMutation{}, err
	}
	// A package can pass its own validation and still be kept out of the
	// catalog — the reload is the only place that knows. Reporting that here
	// keeps the upload from looking successful when nothing was added.
	if reason, failed := r.packageError(mutation.ID); failed {
		return svc.PackageMutation{}, fmt.Errorf("%w: %s", svc.ErrPackageInvalid, reason)
	}
	return mutation, nil
}

// reservePackageID refuses an id the binary already defines. Allowing it would
// mean an upload could redefine what a built-in application installs, which is
// a much larger claim than "add an application". It is the same role the
// catalog loader's own reserve callback plays, spelled the same way.
func (r *Registry) reservePackageID(id string) error {
	if r.applicationIsBuiltin(id) {
		return errPackageReserved(id)
	}
	return nil
}

// RemovePackage deletes a stored package and reloads the catalog.
//
// The store is asked first, and a built-in id is refused only when nothing is
// stored under it. The two are not the same question: a release that builds in
// an application people had uploaded leaves their files on disk, shadowed and
// inert, and refusing on the id alone would make those the one kind of package
// that can never be removed.
func (r *Registry) RemovePackage(id string) error {
	if r.packages == nil {
		return svc.ErrPackagesUnavailable
	}
	if err := r.packages.remove(id); err != nil {
		if errors.Is(err, svc.ErrPackageNotFound) && r.applicationIsBuiltin(id) {
			return errPackageReserved(id)
		}
		return err
	}
	return r.Reload()
}

// applicationIsBuiltin reports whether the catalog entry with this id came from the
// binary rather than from an upload.
func (r *Registry) applicationIsBuiltin(id string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	application, ok := r.view.byID[id]
	return ok && application.Source == svc.SourceBuiltin
}

func (r *Registry) packageError(id string) (string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	reason, failed := r.packageErrors[id]
	return reason, failed
}
