package applications

import (
	"errors"
	"fmt"

	svc "github.com/futrx-com/remote.futrx.com/internal/service/applications"
)

// The registry is what joins an uploaded package to the running catalog: the
// store owns the bytes on disk, and every write to it is followed by a reload
// so the installer, the plugin host and the HTTP handlers see the new entry
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
// the built-in image is being served and these files are not — so the reason
// shown beside it in the package list says that, and says they can go.
func errPackageSuperseded(id string) error {
	return fmt.Errorf("%w: %q", svc.ErrPackageSuperseded, id)
}

// Packages lists the stored packages, annotating each with the reason it is
// not in the catalog when it failed to load.
func (r *Registry) Packages() []svc.Package {
	if r.packages == nil {
		return nil
	}
	stored := r.packages.Packages()

	r.mu.RLock()
	failures := make(map[string]string, len(r.packageErrors))
	for id, reason := range r.packageErrors {
		failures[id] = reason
	}
	r.mu.RUnlock()

	for i := range stored {
		if reason, failed := failures[stored[i].ID]; failed {
			stored[i].Error = reason
			continue
		}
		// What the app *is* — its name, version, kind and the scopes it may be
		// installed at — is whatever the catalog loaded from the files on disk,
		// not whatever was recorded when it was uploaded. Reading it back from
		// the catalog is what keeps the management list from describing a
		// package by a stale copy of its own manifest.
		if img, ok := r.Get(stored[i].ID); ok {
			stored[i].Name = img.Name
			stored[i].Version = img.Version
			stored[i].Type = img.Type
			stored[i].Scopes = img.Scopes
		}
	}
	return stored
}

// InstallPackage stores an uploaded archive and reloads the catalog.
func (r *Registry) InstallPackage(upload svc.PackageUpload) (svc.Package, error) {
	if r.packages == nil {
		return svc.Package{}, svc.ErrPackagesUnavailable
	}
	pkg, err := r.packages.install(upload, r.acceptPackageID)
	if err != nil {
		return svc.Package{}, err
	}
	if err := r.Reload(); err != nil {
		return svc.Package{}, err
	}
	// A package can pass its own validation and still be kept out of the
	// catalog — the reload is the only place that knows. Reporting that here
	// keeps the upload from looking successful when nothing was added.
	if reason, failed := r.packageError(pkg.ID); failed {
		return svc.Package{}, fmt.Errorf("%w: %s", svc.ErrPackageInvalid, reason)
	}
	return pkg, nil
}

// acceptPackageID refuses an id the binary already defines. Allowing it would
// mean an upload could redefine what a built-in application installs, which is
// a much larger claim than "add an application".
func (r *Registry) acceptPackageID(id string) error {
	if r.imageIsBuiltin(id) {
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
	if err := r.packages.RemovePackage(id); err != nil {
		if errors.Is(err, svc.ErrPackageNotFound) && r.imageIsBuiltin(id) {
			return errPackageReserved(id)
		}
		return err
	}
	return r.Reload()
}

// imageIsBuiltin reports whether the catalog entry with this id came from the
// binary rather than from an upload.
func (r *Registry) imageIsBuiltin(id string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	img, ok := r.view.byID[id]
	return ok && img.Source == svc.SourceBuiltin
}

func (r *Registry) packageError(id string) (string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	reason, failed := r.packageErrors[id]
	return reason, failed
}
