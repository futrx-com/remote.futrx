// File Manager's host backend. Its routes reach the project container through
// one fixed container program instead of a web server in the container.
package main

import (
	appAPI "futrx.local/catalog/applications/file-manager/backend/api"
	"futrx.local/catalog/applications/file-manager/backend/config"
	"futrx.local/catalog/applications/file-manager/backend/containerio"
	"futrx.local/catalog/applications/file-manager/backend/service/archives"

	"github.com/futrx-com/remote.futrx.com/pkg/applications/rpc"
)

func main() {
	downloads := archives.NewDownloads(config.MaxConcurrentArchives, config.ArchiveTimeout)
	rpc.Serve(appAPI.New(containerio.Run, downloads))
}
