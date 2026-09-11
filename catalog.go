// Package catalog embeds the installable image catalog that ships with the
// server.
//
// It is a module of its own, and it exists for one reason: go:embed reaches
// only files at or below the directory holding the directive, and it does not
// follow symlinks. Keeping the catalog at images/ — where someone adding an
// app finds it without first learning the backend's package layout — therefore
// means the directive embedding it lives here, at the repository root. The
// backend depends on this module through a local replace; it is never
// published, and futrx.local is not a real host.
package catalog

import "embed"

// FS is the catalog compiled into the server binary. Every image is
// images/<id>, which is also the layout an uploaded package carries, so the
// same loader reads both.
//
//go:embed images
var FS embed.FS
