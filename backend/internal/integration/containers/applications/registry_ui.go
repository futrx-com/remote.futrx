package applications

import (
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"

	svc "github.com/futrx-com/remote.futrx.com/internal/service/applications"
)

const defaultUIEntry = "scripts/main.js"

// loadImageUI resolves an image's ui/ directory into a validated descriptor.
// The directory opts the image in; layout conventions fill fields omitted from
// image.json, while explicitly declared paths always win.
func loadImageUI(fsys fs.FS, root string, declared *svc.ImageUI) (*svc.ImageUI, error) {
	if _, err := fs.Stat(fsys, root); err != nil {
		if declared != nil {
			return nil, fmt.Errorf("image.json declares ui but %s does not exist", root)
		}
		return nil, nil
	}

	ui := svc.ImageUI{}
	if declared != nil {
		ui = *declared
	}
	discoverImageUI(fsys, root, &ui)
	if ui.Entry == "" && len(ui.Styles) == 0 && len(ui.Views) == 0 && !hasAnyFile(fsys, root) {
		return nil, fmt.Errorf("%s exists but is empty", root)
	}
	if err := validateImageUI(fsys, root, ui); err != nil {
		return nil, err
	}
	return &ui, nil
}

func discoverImageUI(fsys fs.FS, root string, ui *svc.ImageUI) {
	if ui.Entry == "" && pathExists(fsys, path.Join(root, defaultUIEntry)) {
		ui.Entry = defaultUIEntry
	}
	if ui.Styles == nil {
		ui.Styles = discoverUIAssets(fsys, root, "style", ".css")
	}
	if ui.Views != nil {
		return
	}
	ui.Views = map[string]string{}
	for _, rel := range discoverUIAssets(fsys, root, "views", ".html") {
		ui.Views[strings.TrimSuffix(path.Base(rel), ".html")] = rel
	}
}

func validateImageUI(fsys fs.FS, root string, ui svc.ImageUI) error {
	check := func(field, rel string) error {
		clean, ok := cleanUIPath(rel)
		if !ok {
			return fmt.Errorf("%s: invalid path %q", field, rel)
		}
		if !pathExists(fsys, path.Join(root, clean)) {
			return fmt.Errorf("%s: %s not found", field, rel)
		}
		return nil
	}
	if ui.Entry != "" {
		if err := check("entry", ui.Entry); err != nil {
			return err
		}
	}
	for _, style := range ui.Styles {
		if err := check("styles", style); err != nil {
			return err
		}
	}
	for name, view := range ui.Views {
		if err := check("views["+name+"]", view); err != nil {
			return err
		}
	}
	return nil
}

func pathExists(fsys fs.FS, name string) bool {
	_, err := fs.Stat(fsys, name)
	return err == nil
}

// hasAnyFile permits asset-only UI directories while rejecting empty ones.
func hasAnyFile(fsys fs.FS, root string) bool {
	found := false
	_ = fs.WalkDir(fsys, root, func(_ string, entry fs.DirEntry, err error) error {
		if err != nil || found {
			return err
		}
		found = !entry.IsDir()
		return nil
	})
	return found
}

// discoverUIAssets returns matching direct children in deterministic order.
func discoverUIAssets(fsys fs.FS, root, directory, extension string) []string {
	entries, err := fs.ReadDir(fsys, path.Join(root, directory))
	if err != nil {
		return nil
	}
	assets := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), extension) {
			continue
		}
		assets = append(assets, path.Join(directory, entry.Name()))
	}
	sort.Strings(assets)
	return assets
}

// cleanUIPath normalizes a path and rejects anything outside an image's ui/.
func cleanUIPath(relativePath string) (string, bool) {
	relativePath = strings.TrimSpace(relativePath)
	if relativePath == "" || strings.HasPrefix(relativePath, "/") || strings.Contains(relativePath, "\\") {
		return "", false
	}
	clean := path.Clean(relativePath)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", false
	}
	return clean, true
}

// UIAsset returns a file from an image's ui/ directory without permitting
// traversal into the rest of the embedded catalog.
func (r *Registry) UIAsset(imageID, assetPath string) ([]byte, bool) {
	img, ok := r.byID[imageID]
	if !ok || img.UI == nil {
		return nil, false
	}
	clean, ok := cleanUIPath(assetPath)
	if !ok {
		return nil, false
	}
	data, err := catalogFS.ReadFile(path.Join("images", imageID, "ui", clean))
	if err != nil {
		return nil, false
	}
	return data, true
}
