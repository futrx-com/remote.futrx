// Package applications loads the installable application catalog and realizes app
// instances inside LXD containers. The catalog under the repository's applications/
// directory is embedded into the binary so a deployed server needs no extra
// files; a server configured with a package store serves those applications plus
// every package an administrator has uploaded into its state directory.
package applications

import (
	"fmt"
	"io/fs"
	"path"
	"sort"
	"sync"

	"futrx.local/catalog"

	svc "github.com/futrx-com/remote.futrx.com/internal/service/applications"
)

// Registry is an in-memory, validated view of an application catalog.
//
// It is a live view rather than a snapshot taken at boot: uploading or
// removing a package reloads it in place, so the installer, the application backend host
// and the HTTP handlers all see the new catalog without being rebuilt. Every
// read is therefore taken under a lock.
type Registry struct {
	// base is the catalog compiled into the binary. It always loads, and a
	// failure to load it is a build error rather than an operational one.
	base fs.FS
	// packages holds uploaded packages, or nil on a server that does not
	// accept uploads.
	packages *PackageStore

	mu   sync.RWMutex
	view catalogView
	// packageErrors records, per package id, why a stored package is not in
	// the catalog. It is reported rather than logged and forgotten, because
	// the files are still on disk and only an operator can act on them.
	packageErrors map[string]string
}

// catalogView is one immutable snapshot of a loaded catalog. Swapping the
// whole struct under the lock is what keeps a reload atomic: a reader never
// sees an application whose script or asset filesystem has not landed yet.
type catalogView struct {
	byID map[string]svc.Application
	// sources maps application ID -> the filesystem it was loaded from, so assets
	// and backend source are read from the right catalog once more than one is
	// in play.
	sources map[string]fs.FS
	// scripts maps application ID -> install script bytes, including payload staging
	// when needed.
	scripts map[string][]byte
	sorted  []svc.Application
}

func newCatalogView() catalogView {
	return catalogView{
		byID:    map[string]svc.Application{},
		sources: map[string]fs.FS{},
		scripts: map[string][]byte{},
	}
}

// EmbeddedCatalog is the application catalog compiled into the binary. It is embedded
// by the module at the repository root rather than here, because go:embed
// cannot reach outside its own directory and the catalog is kept at applications/,
// where an app author finds it.
func EmbeddedCatalog() fs.FS { return catalog.FS }

// NewRegistry loads and validates every applications/<id>/application.json in the
// given catalog, plus — when packages is non-nil — every uploaded package
// stored beside it.
//
// Taking the catalog as an argument is what keeps it a *set of applications*
// rather than a fixed list: the server loads the embedded one, and anything
// else — a test fixture, a catalog assembled from uploaded packages — goes
// through exactly the same validation.
//
// The two halves are held to different standards on purpose. A built-in application
// that does not load is a broken build and fails startup. An uploaded package
// that does not load is one administrator's file, possibly written against a
// different version of Remote: it is skipped with its reason recorded, because
// refusing to boot the whole server over it would turn one bad upload into an
// outage.
func NewRegistry(catalog fs.FS, packages *PackageStore) (*Registry, error) {
	r := &Registry{base: catalog, packages: packages}
	if err := r.Reload(); err != nil {
		return nil, err
	}
	return r, nil
}

// Reload rebuilds the catalog from the built-in applications and the package store.
// It returns an error only when the built-in catalog itself is unloadable;
// per-package failures are recorded and reported through Packages. A failure
// to read the package directory at all is neither: it is swallowed here, and
// the empty listing it also produces is all the caller sees.
func (r *Registry) Reload() error {
	view := newCatalogView()
	if _, err := loadCatalogInto(&view, r.base, svc.SourceBuiltin, nil); err != nil {
		return err
	}
	failures := map[string]string{}
	if r.packages != nil {
		builtin := make(map[string]bool, len(view.byID))
		for id := range view.byID {
			builtin[id] = true
		}
		// A package may not shadow a built-in application. Letting it would mean an
		// upload could redefine what "mysql" installs on a server, which is a
		// far larger claim than "add an application".
		//
		// Reaching here means the files were stored before that id was built
		// in — an upload cannot get past the check made when it is written —
		// so what the operator needs told is what became of their package, not
		// that something was refused.
		reserve := func(id string) error {
			if builtin[id] {
				return errPackageSuperseded(id)
			}
			return nil
		}
		// loadCatalogInto only returns an error here if the packages directory
		// itself is unreadable, which is a store problem rather than a package
		// problem; skipped is nil then and the loop below does nothing.
		skipped, _ := loadCatalogInto(&view, r.packages.FS(), svc.SourceUploaded, reserve)
		for id, reason := range skipped {
			failures[id] = reason
		}
	}
	sortCatalog(&view)

	r.mu.Lock()
	r.view = view
	r.packageErrors = failures
	r.mu.Unlock()
	return nil
}

// loadCatalogInto reads every applications/<id> in catalog and adds it to view. It
// returns the reason each application was left out, keyed by id.
//
// reserve is nil for the built-in catalog and non-nil for uploaded packages:
// when it is set, an application it rejects — or one that fails validation — is
// skipped with its reason reported instead of failing the whole load.
func loadCatalogInto(view *catalogView, catalog fs.FS, source svc.ApplicationSource, reserve func(string) error) (map[string]string, error) {
	entries, err := fs.ReadDir(catalog, catalogRoot)
	if err != nil {
		return nil, fmt.Errorf("read application catalog: %w", err)
	}
	var skipped map[string]string
	skip := func(id string, err error) {
		if skipped == nil {
			skipped = map[string]string{}
		}
		skipped[id] = err.Error()
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		id := e.Name()
		if isCatalogMetadataDirectory(id) {
			continue
		}
		if reserve != nil {
			if err := reserve(id); err != nil {
				skip(id, err)
				continue
			}
		}
		load := loadApplication
		if source == svc.SourceUploaded {
			// Stored uploads predate the strict manifest decoder and deliberately
			// survive Remote upgrades. Keep the old encoding/json compatibility
			// rules for those already-committed files; PackageStore.add validates
			// every new or replacement upload strictly before publishing it.
			load = loadPersistedApplication
		}
		application, script, err := load(catalog, id)
		if err != nil {
			err = fmt.Errorf("load application %q: %w", id, err)
			if reserve == nil {
				return nil, err
			}
			skip(id, err)
			continue
		}
		application.Source = source
		view.byID[id] = application
		view.sources[id] = catalog
		view.scripts[id] = script
		view.sorted = append(view.sorted, application)
	}
	return skipped, nil
}

func sortCatalog(view *catalogView) {
	sort.Slice(view.sorted, func(i, j int) bool { return view.sorted[i].Name < view.sorted[j].Name })
}

func loadApplication(catalog fs.FS, id string) (svc.Application, []byte, error) {
	return loadApplicationManifest(catalog, id, false)
}

// loadPersistedApplication preserves the decoder behavior that was in force
// when an older uploaded package was accepted. Recognized values still pass
// all current semantic validation and every derived field is recomputed below.
func loadPersistedApplication(catalog fs.FS, id string) (svc.Application, []byte, error) {
	return loadApplicationManifest(catalog, id, true)
}

func loadApplicationManifest(
	catalog fs.FS,
	id string,
	legacyJSONCompatibility bool,
) (svc.Application, []byte, error) {
	root := path.Join(catalogRoot, id)
	readManifest := readApplicationManifest
	if legacyJSONCompatibility {
		readManifest = readPersistedApplicationManifest
	}
	raw, err := readManifest(catalog, path.Join(root, "application.json"))
	if err != nil {
		return svc.Application{}, nil, fmt.Errorf("read application.json: %w", err)
	}
	var application svc.Application
	if legacyJSONCompatibility {
		err = decodePersistedApplicationManifest(raw, &application)
	} else {
		err = decodeApplicationManifest(raw, &application)
	}
	if err != nil {
		return svc.Application{}, nil, fmt.Errorf("parse application.json: %w", err)
	}
	if application.ID == "" {
		application.ID = id
	}
	if application.ID != id {
		return svc.Application{}, nil, fmt.Errorf("application id %q does not match directory %q", application.ID, id)
	}
	if !packageIDPattern.MatchString(application.ID) {
		return svc.Application{}, nil, fmt.Errorf(
			"application id %q must be lowercase letters, digits and dashes, starting with a letter or digit",
			application.ID,
		)
	}
	// Container metadata is derived from backend/container/ below. A manifest
	// cannot claim a build identity or commands that the package does not carry.
	application.Container = nil
	if err := validateInstallScriptPath(application.Install); err != nil {
		return svc.Application{}, nil, err
	}
	ui, err := loadApplicationUI(catalog, path.Join(root, "ui"), application.UI)
	if err != nil {
		return svc.Application{}, nil, fmt.Errorf("ui: %w", err)
	}
	application.UI = ui

	backend, err := loadApplicationBackend(catalog, path.Join(root, backendDir), application.Backend)
	if err != nil {
		return svc.Application{}, nil, fmt.Errorf("backend: %w", err)
	}
	application.Backend = backend

	skills, err := loadApplicationSkills(catalog, path.Join(root, skillsDir))
	if err != nil {
		return svc.Application{}, nil, fmt.Errorf("skills: %w", err)
	}
	application.Skills = skills

	infrastructure, err := loadApplicationInfrastructure(
		catalog, root, application.ID, application.Version, application.Install)
	if err != nil {
		return svc.Application{}, nil, err
	}
	application.Install = infrastructure.installPath
	application.Container = infrastructure.container
	if err := validateApplication(application); err != nil {
		return svc.Application{}, nil, err
	}
	return application, infrastructure.script, nil
}

// List returns the catalog sorted by display name.
func (r *Registry) List() []svc.Application {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]svc.Application, len(r.view.sorted))
	for i, application := range r.view.sorted {
		out[i] = cloneApplication(application)
	}
	return out
}

// Get returns the application with the given ID.
func (r *Registry) Get(id string) (svc.Application, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	application, ok := r.view.byID[id]
	if !ok {
		return svc.Application{}, false
	}
	return cloneApplication(application), true
}

// Script returns the install script bytes for an application ID.
func (r *Registry) Script(id string) ([]byte, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.view.scripts[id]
	return s, ok
}

// applicationSource returns the application and the filesystem it was loaded from.
func (r *Registry) applicationSource(id string) (svc.Application, fs.FS, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	application, ok := r.view.byID[id]
	if !ok {
		return svc.Application{}, nil, false
	}
	return application, r.view.sources[id], true
}

var _ svc.Registry = (*Registry)(nil)

// catalogRoot is the directory every catalog filesystem holds its applications in.
const catalogRoot = "applications"

// isCatalogMetadataDirectory identifies directories that may sit beside applications
// without being one. Every other directory is validated as an application and fails
// the catalog if it does not parse, so the list is intentionally closed: it is
// the escape hatch for catalog-level material, not a place to put an application.
// The name is also refused as an uploaded package id, so a package can never
// shadow one.
func isCatalogMetadataDirectory(name string) bool {
	return name == "docs"
}
