package applications

import (
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"path"
	"strings"

	svc "github.com/futrx-com/remote.futrx.com/internal/service/applications"
)

// pluginDir is the fixed directory an image ships its Go backend in. Like
// ui/, the directory is the opt-in: image.json's backend block only overrides
// defaults, so a plugin cannot be declared without shipping one.
const pluginDir = "plugin"

// loadImagePlugin resolves an image's plugin/ directory into a validated
// descriptor. It checks the shape the build depends on — that there is Go
// source, that it is a program rather than a library, and that it does not
// carry its own module — because every one of those failures would otherwise
// surface as a compiler error on a production server at install time.
func loadImagePlugin(fsys fs.FS, root string, declared *svc.ImageBackend) (*svc.ImageBackend, error) {
	if _, err := fs.Stat(fsys, root); err != nil {
		if declared != nil {
			return nil, fmt.Errorf("image.json declares backend but %s does not exist", root)
		}
		return nil, nil
	}

	backend := svc.ImageBackend{}
	if declared != nil {
		backend = *declared
	}
	if backend.Access != "" && !backend.Access.Valid() {
		return nil, fmt.Errorf("invalid access %q", backend.Access)
	}
	if backend.TimeoutMS < 0 {
		return nil, fmt.Errorf("timeoutMs must not be negative")
	}
	if err := validatePluginSource(fsys, root); err != nil {
		return nil, err
	}
	return &backend, nil
}

// validatePluginSource enforces what the build directory the server generates
// can actually compile: package main at the root of plugin/, and no module
// file of its own, since the server writes one that pins the SDK.
func validatePluginSource(fsys fs.FS, root string) error {
	entries, err := fs.ReadDir(fsys, root)
	if err != nil {
		return fmt.Errorf("read %s: %w", root, err)
	}
	mainFiles := 0
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			continue
		}
		if name == "go.mod" || name == "go.sum" {
			return fmt.Errorf(
				"%s/%s is not supported: the server generates the plugin module", root, name)
		}
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		pkg, err := packageName(fsys, path.Join(root, name))
		if err != nil {
			return err
		}
		if pkg != "main" {
			return fmt.Errorf("%s/%s declares package %q, want main", root, name, pkg)
		}
		mainFiles++
	}
	if mainFiles == 0 {
		return fmt.Errorf("%s contains no package main source", root)
	}
	return nil
}

func packageName(fsys fs.FS, name string) (string, error) {
	source, err := fs.ReadFile(fsys, name)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", name, err)
	}
	file, err := parser.ParseFile(token.NewFileSet(), name, source, parser.PackageClauseOnly)
	if err != nil {
		return "", fmt.Errorf("parse %s: %w", name, err)
	}
	return file.Name.Name, nil
}

// PluginSource returns the Go source under an image's plugin/ directory,
// rooted at that directory. These bytes are compiled by the plugin host; unlike
// ui/ assets they are never served, so there is no path-traversal surface here
// — a caller gets the whole subtree or nothing.
func (r *Registry) PluginSource(imageID string) (fs.FS, bool) {
	img, ok := r.byID[imageID]
	if !ok || img.Backend == nil {
		return nil, false
	}
	sub, err := fs.Sub(catalogFS, path.Join("images", imageID, pluginDir))
	if err != nil {
		return nil, false
	}
	return sub, true
}
