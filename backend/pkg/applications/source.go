package applications

import (
	"embed"
	"io/fs"
)

// ModulePath is the Go module the SDK belongs to, and ImportPath is where this
// package sits inside it. A backend's source imports it by that canonical path,
// so backend code under applications/<id>/backend/ compiles in an editor
// against the repository as well as it does inside the server's generated
// module.
const (
	ModulePath = "github.com/futrx-com/remote.futrx.com"
	ImportPath = ModulePath + "/pkg/applications"
	// PackageDir is where Source's files belong relative to a module root.
	PackageDir = "pkg/applications"
)

// The SDK is embedded in the server binary because that is the only way a
// backend's source can be compiled on a host that has no checkout of this
// repository. The catalog ships source, not binaries; the server materializes
// this package beside an application's host backend module and builds
// backend/api plus any imported sibling packages together.
//
// The list is explicit rather than a *.go glob so that test files stay out of
// what a backend compiles against. TestSourceCoversEveryFile keeps it complete.
//
//go:embed contract.go events.go router.go request.go response.go source.go rpc/client.go rpc/rpc.go rpc/server.go rpc/stream.go rpc/wire.go
var sdkSource embed.FS

// Source returns the SDK's own Go source, rooted at this package's directory:
// "contract.go", "rpc/rpc.go", and so on.
func Source() fs.FS { return sdkSource }
