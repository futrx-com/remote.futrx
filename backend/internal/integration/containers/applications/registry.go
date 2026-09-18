// Package applications loads the installable application catalog and realizes app
// instances inside LXD containers. The catalog under the repository's applications/
// directory is embedded into the binary so a deployed server needs no extra
// files; a server configured with a package store serves those applications plus
// every package an administrator has uploaded into its state directory.
package applications

import (
	"encoding/json"
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
// removing a package reloads it in place, so the installer, the plugin host
// and the HTTP handlers all see the new catalog without being rebuilt. Every
// read is therefore taken under a lock.
type Registry struct {
	// base is the catalog compiled into the binary. It always loads, and a
	// failure to load it is a build error rather than an operational one.
	base fs.FS

	mu   sync.RWMutex
	view catalogView
}

// catalogView is one immutable snapshot of a loaded catalog. Swapping the
// whole struct under the lock is what keeps a reload atomic: a reader never
// sees an application whose script or asset filesystem has not landed yet.
type catalogView struct {
	byID map[string]svc.Application
	// sources maps application ID -> the filesystem it was loaded from, so assets
	// and plugin source are read from the right catalog once more than one is
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

// NewRegistry loads and validates the catalog embedded in the binary.
func NewRegistry() (*Registry, error) { return NewRegistryFromFS(catalog.FS) }

// NewRegistryFromFS loads and validates every applications/<id>/application.json in the
// given filesystem. A malformed entry is a build/asset error, so loading fails
// loudly rather than silently dropping an app.
//
// Taking the filesystem as an argument is what keeps the catalog a *set of
// applications* rather than a fixed list: the server loads the embedded one, and
// anything else — a test fixture, a catalog assembled from uploaded packages —
// goes through exactly the same validation.
func NewRegistryFromFS(catalog fs.FS) (*Registry, error) {
	r := &Registry{base: catalog}
	if err := r.Reload(); err != nil {
		return nil, err
	}
	return r, nil
}

// Reload rebuilds the catalog from the built-in applications. It returns an error
// when the catalog is unloadable, which on a built-in catalog is a broken
// build rather than an operational problem.
func (r *Registry) Reload() error {
	view := newCatalogView()
	if _, err := loadCatalogInto(&view, r.base, svc.SourceBuiltin, nil); err != nil {
		return err
	}
	sortCatalog(&view)

	r.mu.Lock()
	r.view = view
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
		application, script, err := loadApplication(catalog, id)
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
	root := path.Join(catalogRoot, id)
	raw, err := fs.ReadFile(catalog, path.Join(root, "application.json"))
	if err != nil {
		return svc.Application{}, nil, fmt.Errorf("read application.json: %w", err)
	}
	var application svc.Application
	if err := json.Unmarshal(raw, &application); err != nil {
		return svc.Application{}, nil, fmt.Errorf("parse application.json: %w", err)
	}
	if application.ID == "" {
		application.ID = id
	}
	if application.ID != id {
		return svc.Application{}, nil, fmt.Errorf("application id %q does not match directory %q", application.ID, id)
	}
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

	var script []byte
	application.Install, script, err = loadApplicationInfrastructure(catalog, root, application.Install)
	if err != nil {
		return svc.Application{}, nil, err
	}
	if err := validateApplication(application); err != nil {
		return svc.Application{}, nil, err
	}
	return application, script, nil
}

// List returns the catalog sorted by display name.
func (r *Registry) List() []svc.Application {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]svc.Application, len(r.view.sorted))
	copy(out, r.view.sorted)
	return out
}

// Get returns the application with the given ID.
func (r *Registry) Get(id string) (svc.Application, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	application, ok := r.view.byID[id]
	return application, ok
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
