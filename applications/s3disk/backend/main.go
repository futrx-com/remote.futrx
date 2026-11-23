// S3Disk's host composition root. Remote builds this process from backend/.
package main

import (
	appAPI "futrx.local/catalog/applications/s3disk/backend/api"
	appLifecycle "futrx.local/catalog/applications/s3disk/backend/lifecycle"
	"github.com/futrx-com/remote.futrx.com/pkg/applications"
	"github.com/futrx-com/remote.futrx.com/pkg/applications/rpc"
)

func main() {
	rpc.ServeWithRuntime(func(runtime applications.Runtime) applications.Backend {
		return appAPI.New(
			appLifecycle.NewOperations(),
			appLifecycle.NewPushes(runtime.Events),
		)
	})
}
