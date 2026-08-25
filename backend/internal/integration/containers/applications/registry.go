// Package applications loads the installable image catalog and realizes app
// instances inside LXD containers. The catalog under images/ is embedded into
// the binary so a deployed server needs no extra files.
package applications

import (
	"embed"
	"fmt"
	"io/fs"
	"path"
	"sort"

	"encoding/json"

	svc "github.com/futrx-com/remote.futrx.com/internal/service/applications"
)

//go:embed images
var catalogFS embed.FS

// Registry is an in-memory, validated view of the embedded image catalog.
type Registry struct {
	byID   map[string]svc.Image
	sorted []svc.Image
	// scripts maps image ID -> raw install script bytes.
	scripts map[string][]byte
}

// NewRegistry loads and validates every images/<id>/image.json. A malformed
// entry is a build/asset error, so loading fails loudly rather than silently
// dropping an app.
func NewRegistry() (*Registry, error) {
	entries, err := fs.ReadDir(catalogFS, "images")
	if err != nil {
		return nil, fmt.Errorf("read image catalog: %w", err)
	}
	r := &Registry{byID: map[string]svc.Image{}, scripts: map[string][]byte{}}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		id := e.Name()
		img, script, err := loadImage(id)
		if err != nil {
			return nil, fmt.Errorf("load image %q: %w", id, err)
		}
		r.byID[id] = img
		r.scripts[id] = script
		r.sorted = append(r.sorted, img)
	}
	sort.Slice(r.sorted, func(i, j int) bool { return r.sorted[i].Name < r.sorted[j].Name })
	return r, nil
}

func loadImage(id string) (svc.Image, []byte, error) {
	raw, err := catalogFS.ReadFile(path.Join("images", id, "image.json"))
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
	if err := validate(img); err != nil {
		return svc.Image{}, nil, err
	}
	if img.Install == "" {
		img.Install = "install.sh"
	}
	script, err := catalogFS.ReadFile(path.Join("images", id, img.Install))
	if err != nil {
		return svc.Image{}, nil, fmt.Errorf("read install script %q: %w", img.Install, err)
	}
	return img, script, nil
}

func validate(img svc.Image) error {
	if img.Name == "" {
		return fmt.Errorf("missing name")
	}
	if len(img.Scopes) == 0 {
		return fmt.Errorf("missing scopes")
	}
	for _, s := range img.Scopes {
		if !s.Valid() {
			return fmt.Errorf("invalid scope %q", s)
		}
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
	out := make([]svc.Image, len(r.sorted))
	copy(out, r.sorted)
	return out
}

// Get returns the image with the given ID.
func (r *Registry) Get(id string) (svc.Image, bool) {
	img, ok := r.byID[id]
	return img, ok
}

// Script returns the install script bytes for an image ID.
func (r *Registry) Script(id string) ([]byte, bool) {
	s, ok := r.scripts[id]
	return s, ok
}

var _ svc.Registry = (*Registry)(nil)
