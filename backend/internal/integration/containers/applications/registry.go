// Package applications loads the installable image catalog and realizes app
// instances inside LXD containers. The catalog under the repository's images/
// directory is embedded into the binary so a deployed server needs no extra
// files; a server configured with a package store serves those images plus
// every package an administrator has uploaded into its state directory.
package applications

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"
	"sync"

	"futrx.local/catalog"

	"github.com/futrx-com/remote.futrx.com/internal/integration/hosttools"
	svc "github.com/futrx-com/remote.futrx.com/internal/service/applications"
)

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

	mu   sync.RWMutex
	view catalogView
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
}

func newCatalogView() catalogView {
	return catalogView{
		byID:    map[string]svc.Image{},
		sources: map[string]fs.FS{},
		scripts: map[string][]byte{},
	}
}

// EmbeddedCatalog is the image catalog compiled into the binary. It is embedded
// by the module at the repository root rather than here, because go:embed
// cannot reach outside its own directory and the catalog is kept at images/,
// where an app author finds it.
func EmbeddedCatalog() fs.FS { return catalog.FS }

// NewRegistry loads and validates the catalog embedded in the binary.
func NewRegistry() (*Registry, error) { return NewRegistryFromFS(catalog.FS) }

// NewRegistryFromFS loads and validates every images/<id>/image.json in the
// given filesystem. A malformed entry is a build/asset error, so loading fails
// loudly rather than silently dropping an app.
//
// Taking the filesystem as an argument is what keeps the catalog a *set of
// images* rather than a fixed list: the server loads the embedded one, and
// anything else — a test fixture, a catalog assembled from uploaded packages —
// goes through exactly the same validation.
func NewRegistryFromFS(catalog fs.FS) (*Registry, error) {
	r := &Registry{base: catalog}
	if err := r.Reload(); err != nil {
		return nil, err
	}
	return r, nil
}

// Reload rebuilds the catalog from the built-in images. It returns an error
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

// loadCatalogInto reads every images/<id> in catalog and adds it to view. It
// returns the reason each image was left out, keyed by id.
//
// reserve is nil for the built-in catalog and non-nil for uploaded packages:
// when it is set, an image it rejects — or one that fails validation — is
// skipped with its reason reported instead of failing the whole load.
func loadCatalogInto(view *catalogView, catalog fs.FS, source svc.ImageSource, reserve func(string) error) (map[string]string, error) {
	entries, err := fs.ReadDir(catalog, catalogRoot)
	if err != nil {
		return nil, fmt.Errorf("read image catalog: %w", err)
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
		img, script, err := loadImage(catalog, id)
		if err != nil {
			err = fmt.Errorf("load image %q: %w", id, err)
			if reserve == nil {
				return nil, err
			}
			skip(id, err)
			continue
		}
		img.Source = source
		view.byID[id] = img
		view.sources[id] = catalog
		view.scripts[id] = script
		view.sorted = append(view.sorted, img)
	}
	return skipped, nil
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

	// A backend image installs nothing in a container, so it has no install
	// script to read: its plugin/ directory is the whole payload, and it must
	// actually carry it. A tool does reach a container, so it falls through and
	// its script is loaded.
	if !img.Type.NeedsContainer() {
		if img.Type == svc.KindBackend && img.Backend == nil {
			return svc.Image{}, nil, fmt.Errorf("type %q requires a %s/ directory", img.Type, pluginDir)
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
		// it in: it is what stop and uninstall act on. A tool has one; a
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

// isCatalogMetadataDirectory identifies directories that may sit beside images
// without being one. Every other directory is validated as an image and fails
// the catalog if it does not parse, so the list is intentionally closed: it is
// the escape hatch for catalog-level material, not a place to put an image.
// The name is also refused as an uploaded package id, so a package can never
// shadow one.
func isCatalogMetadataDirectory(name string) bool {
	return name == "docs"
}
