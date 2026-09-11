package appplugin

import (
	"embed"
	"io/fs"
)

// ModulePath is the Go module the SDK belongs to, and ImportPath is where this
// package sits inside it. A plugin's source imports it by that canonical path,
// so plugin code under images/<id>/plugin/ compiles in an editor against the
// repository as well as it does inside the server's build directory.
const (
	ModulePath = "github.com/futrx-com/remote.futrx.com"
	ImportPath = ModulePath + "/pkg/appplugin"
	// PackageDir is where Source's files belong relative to a module root.
	PackageDir = "pkg/appplugin"
)

// The SDK is embedded in the server binary because that is the only way a
// plugin's source can be compiled on a host that has no checkout of this
// repository. The catalog ships source, not binaries; the server materializes
// this package beside an image's plugin/ directory and builds the two together.
//
// The list is explicit rather than a *.go glob so that test files stay out of
// what a plugin compiles against. TestSourceCoversEveryFile keeps it complete.
//
//go:embed contract.go mux.go request.go response.go source.go pluginrpc/client.go pluginrpc/pluginrpc.go pluginrpc/server.go pluginrpc/wire.go
var sdkSource embed.FS

// Source returns the SDK's own Go source, rooted at this package's directory:
// "contract.go", "pluginrpc/pluginrpc.go", and so on.
func Source() fs.FS { return sdkSource }
