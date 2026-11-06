// File Manager's host backend. Its routes reach the project container through
// one fixed container program instead of a web server in the container.
package main

import (
	appAPI "futrx.local/catalog/applications/file-manager/backend/api"
	"futrx.local/catalog/applications/file-manager/backend/containerio"

	"github.com/futrx-com/remote.futrx.com/pkg/applications/rpc"
)

func main() {
	rpc.Serve(appAPI.New(containerio.Run))
}
