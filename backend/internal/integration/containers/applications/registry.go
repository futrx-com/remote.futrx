// Package applications loads the installable image catalog and realizes app
// instances inside LXD containers. The catalog under images/ is embedded into
// the binary so a deployed server needs no extra files; a server configured
// with a package store serves those images plus every package an
// administrator has uploaded into its state directory.
package applications

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"
	"sync"

	"github.com/futrx-com/remote.futrx.com/internal/integration/hosttools"
	svc "github.com/futrx-com/remote.futrx.com/internal/service/applications"
)

//go:embed images
var catalogFS embed.FS

// Registry is an in-memory, validated view of an image catalog.
//
// It is a live view rather than a snapshot taken at boot: uploading or
// removing a package reloads it in place, so the installer, the plugin host
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
// sees an image whose script or asset filesystem has not landed yet.
type catalogView struct {
	byID map[string]svc.Image
	// sources maps image ID -> the filesystem it was loaded from, so assets
	// and plugin source are read from the right catalog once more than one is
	// in play.
	sources map[string]fs.FS
	// scripts maps image ID -> install script bytes, including payload staging
	// when needed.
	scripts map[string][]byte
	sorted  []svc.Image
	// skipped records why an image was left out, keyed by id. Only a catalog
	// loaded leniently — an uploaded one — ever fills it.
	skipped map[string]string
}

func (v *catalogView) skip(id string, err error) {
	if v.skipped == nil {
		v.skipped = map[string]string{}
	}
	v.skipped[id] = err.Error()
}

func newCatalogView() catalogView {
	return catalogView{
		byID:    map[string]svc.Image{},
		sources: map[string]fs.FS{},
		scripts: map[string][]byte{},
	}
}

// EmbeddedCatalog is the image catalog compiled into the binary.
func EmbeddedCatalog() fs.FS { return catalogFS }

// NewRegistry loads and validates the catalog embedded in the binary.
func NewRegistry() (*Registry, error) { return NewRegistryFromFS(catalogFS) }

// NewRegistryFromFS loads and validates every images/<id>/image.json in the
// given filesystem. A malformed entry is a build/asset error, so loading fails
// loudly rather than silently dropping an app.
//
// Taking the filesystem as an argument is what keeps the catalog a *set of
// images* rather than a fixed list: the server loads the embedded one, and
// anything else — a test fixture, a catalog assembled from uploaded packages —
// goes through exactly the same validation.
func NewRegistryFromFS(catalog fs.FS) (*Registry, error) {
	return NewRegistryWithPackages(catalog, nil)
}

// NewRegistryWithPackages loads the built-in catalog and, when packages is
// non-nil, every uploaded package stored beside it.
//
// The two halves are held to different standards on purpose. A built-in image
// that does not load is a broken build and fails startup. An uploaded package
// that does not load is one administrator's file, possibly written against a
// different version of Remote: it is skipped with its reason recorded, because
// refusing to boot the whole server over it would turn one bad upload into an
// outage.
func NewRegistryWithPackages(catalog fs.FS, packages *PackageStore) (*Registry, error) {
	r := &Registry{base: catalog, packages: packages}
	if err := r.Reload(); err != nil {
		return nil, err
	}
	return r, nil
}

// Reload rebuilds the catalog from the built-in images and the package store.
// It returns an error only when the built-in catalog itself is unloadable;
// per-package failures are recorded and reported through Packages.
func (r *Registry) Reload() error {
	view := newCatalogView()
	if err := loadCatalogInto(&view, r.base, svc.SourceBuiltin, nil); err != nil {
		return err
	}
	failures := map[string]string{}
	if r.packages != nil {
		builtin := make(map[string]bool, len(view.byID))
		for id := range view.byID {
			builtin[id] = true
		}
		// A package may not shadow a built-in image. Letting it would mean an
		// upload could redefine what "mysql" installs on a server, which is a
		// far larger claim than "add an application".
		reserve := func(id string) error {
			if builtin[id] {
				return fmt.Errorf("%w: %q", svc.ErrPackageReserved, id)
			}
			return nil
		}
		if err := loadCatalogInto(&view, r.packages.FS(), svc.SourceUploaded, reserve); err != nil {
			// loadCatalogInto only returns an error here if the packages
			// directory itself is unreadable, which is a store problem rather
			// than a package problem.
			failures[""] = err.Error()
		} else {
			for id, reason := range view.skipped {
				failures[id] = reason
			}
		}
	}
	view.skipped = nil
	sortCatalog(&view)

	r.mu.Lock()
	r.view = view
	r.packageErrors = failures
	r.mu.Unlock()
	return nil
}

// loadCatalogInto reads every images/<id> in catalog and adds it to view.
//
// reserve is nil for the built-in catalog and non-nil for uploaded packages:
// when it is set, an image it rejects — or one that fails validation — is
// skipped with its reason recorded instead of failing the whole load.
func loadCatalogInto(view *catalogView, catalog fs.FS, source svc.ImageSource, reserve func(string) error) error {
	entries, err := fs.ReadDir(catalog, catalogRoot)
	if err != nil {
		return fmt.Errorf("read image catalog: %w", err)
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
				view.skip(id, err)
				continue
			}
		}
		img, script, err := loadImage(catalog, id)
		if err != nil {
			err = fmt.Errorf("load image %q: %w", id, err)
			if reserve == nil {
				return err
			}
			view.skip(id, err)
			continue
		}
		img.Source = source
		view.byID[id] = img
		view.sources[id] = catalog
		view.scripts[id] = script
		view.sorted = append(view.sorted, img)
	}
	return nil
}

func sortCatalog(view *catalogView) {
	sort.Slice(view.sorted, func(i, j int) bool { return view.sorted[i].Name < view.sorted[j].Name })
}

func loadImage(catalog fs.FS, id string) (svc.Image, []byte, error) {
	raw, err := fs.ReadFile(catalog, path.Join(catalogRoot, id, "image.json"))
	if err != nil {
		return svc.Image{}, nil, fmt.Errorf("read image.json: %w", err)
	}
	var img svc.Image
	if err := json.Unmarshal(raw, &img); err != nil {
		return svc.Image{}, nil, fmt.Errorf("parse image.json: %w", err)
	}
	if img.ID == "" {
		img.ID = id
	}
	if img.ID != id {
		return svc.Image{}, nil, fmt.Errorf("image id %q does not match directory %q", img.ID, id)
	}
	if img.Type == "" {
		img.Type = svc.KindService
	}
	if err := validate(img); err != nil {
		return svc.Image{}, nil, err
	}
	ui, err := loadImageUI(catalog, path.Join(catalogRoot, id, "ui"), img.UI)
	if err != nil {
		return svc.Image{}, nil, fmt.Errorf("ui: %w", err)
	}
	img.UI = ui

	backend, err := loadImagePlugin(catalog, path.Join(catalogRoot, id, pluginDir), img.Backend)
	if err != nil {
		return svc.Image{}, nil, fmt.Errorf("backend: %w", err)
	}
	img.Backend = backend

	skills, err := loadImageSkills(catalog, path.Join(catalogRoot, id, skillsDir))
	if err != nil {
		return svc.Image{}, nil, fmt.Errorf("skills: %w", err)
	}
	img.Skills = skills

	// A UI or backend image installs nothing in a container, so it has no
	// install script to read: its ui/ or plugin/ directory is the whole
	// payload. Each kind must actually carry the half it is named for. A tool
	// does reach a container, so it falls through and its script is loaded.
	if !img.Type.NeedsContainer() {
		switch img.Type {
		case svc.KindUI:
			if img.UI == nil {
				return svc.Image{}, nil, fmt.Errorf("type %q requires a ui/ directory", img.Type)
			}
		case svc.KindBackend:
			if img.Backend == nil {
				return svc.Image{}, nil, fmt.Errorf("type %q requires a %s/ directory", img.Type, pluginDir)
			}
		}
		return img, nil, nil
	}

	if img.Install == "" {
		img.Install = "install.sh"
	}
	script, err := fs.ReadFile(catalog, path.Join(catalogRoot, id, img.Install))
	if err != nil {
		return svc.Image{}, nil, fmt.Errorf("read install script %q: %w", img.Install, err)
	}
	script, err = withContainerPayload(catalog, path.Join(catalogRoot, id), script)
	if err != nil {
		return svc.Image{}, nil, fmt.Errorf("container payload: %w", err)
	}
	return img, script, nil
}

func validate(img svc.Image) error {
	if len(img.HostTools) > 0 && !img.Type.NeedsContainer() {
		return fmt.Errorf("host tools require a provisioned image")
	}
	for _, tool := range img.HostTools {
		if err := hosttools.Validate(tool); err != nil {
			return fmt.Errorf("host tool %q: %w", tool.Name, err)
		}
	}

	if img.Name == "" {
		return fmt.Errorf("missing name")
	}
	// Version is what an installed instance is compared against to decide
	// whether its install script has to run again, so an image without one
	// could never be upgraded in place. It is free text — "8.0", "16",
	// "1.2.3-rc1" — because the only question ever asked of it is whether it
	// differs from what an instance recorded, never which of two is newer.
	if strings.TrimSpace(img.Version) == "" {
		return fmt.Errorf("missing version")
	}
	if !img.Type.Valid() {
		return fmt.Errorf("invalid type %q", img.Type)
	}
	if len(img.Scopes) == 0 {
		return fmt.Errorf("missing scopes")
	}
	for _, s := range img.Scopes {
		if !s.Valid() {
			return fmt.Errorf("invalid scope %q", s)
		}
	}
	// Ports and health probes describe something reachable on a port. Accepting
	// them from a kind that exposes nothing would be a lie, since nothing would
	// ever read them.
	if !img.Type.NeedsPort() {
		if img.Port.Internal != 0 || img.Healthcheck.Command != "" {
			return fmt.Errorf("type %q must not declare port or healthcheck", img.Type)
		}
		// A systemd unit is only meaningful where there is a container to run
		// it in: it is what stop and uninstall act on. A tool has one; a UI or
		// backend image has no container at all.
		if !img.Type.NeedsContainer() && img.Service != "" {
			return fmt.Errorf("type %q must not declare service", img.Type)
		}
		if img.Type == svc.KindTool {
			for _, sc := range img.Scopes {
				if sc == svc.ScopeGlobal {
					return fmt.Errorf("type %q supports project scope only", img.Type)
				}
			}
		}
		return nil
	}
	if img.Port.Internal <= 0 {
		return fmt.Errorf("missing port.internal")
	}
	if img.Port.DefaultExternal <= 0 {
		img.Port.DefaultExternal = img.Port.Internal
	}
	return nil
}

// List returns the catalog sorted by display name.
func (r *Registry) List() []svc.Image {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]svc.Image, len(r.view.sorted))
	copy(out, r.view.sorted)
	return out
}

// Get returns the image with the given ID.
func (r *Registry) Get(id string) (svc.Image, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	img, ok := r.view.byID[id]
	return img, ok
}

// Script returns the install script bytes for an image ID.
func (r *Registry) Script(id string) ([]byte, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.view.scripts[id]
	return s, ok
}

// imageSource returns the image and the filesystem it was loaded from.
func (r *Registry) imageSource(id string) (svc.Image, fs.FS, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	img, ok := r.view.byID[id]
	if !ok {
		return svc.Image{}, nil, false
	}
	return img, r.view.sources[id], true
}

var _ svc.Registry = (*Registry)(nil)

// catalogRoot is the directory every catalog filesystem holds its images in.
const catalogRoot = "images"

// isCatalogMetadataDirectory identifies directories embedded beside images
// that describe the catalog itself. Every other directory is validated as an
// image, so this list is intentionally closed and immutable.
func isCatalogMetadataDirectory(name string) bool {
	return name == "docs"
}
