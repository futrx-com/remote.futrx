// File Management's composition root. Request handling and filesystem policy
// live in importable packages so this executable only wires the backend.
package main

import (
	appAPI "futrx.local/catalog/applications/file-management/backend/api"
	"github.com/futrx-com/remote.futrx.com/pkg/applications/rpc"
)

func main() {
	rpc.Serve(appAPI.New())
}
