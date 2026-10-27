package main

import (
	appAPI "futrx.local/catalog/applications/code-server/backend/api"
	"futrx.local/catalog/applications/code-server/backend/containerio"
	"futrx.local/catalog/applications/code-server/backend/settings"

	"github.com/futrx-com/remote.futrx.com/pkg/applications"
	"github.com/futrx-com/remote.futrx.com/pkg/applications/rpc"
)

func main() {
	rpc.ServeWithRuntime(func(_ applications.Runtime) applications.Backend {
		store := settings.New(containerio.ReadSettings, containerio.WriteSettings)
		return appAPI.New(store)
	})
}
