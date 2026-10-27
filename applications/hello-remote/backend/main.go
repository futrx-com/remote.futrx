// Hello Remote is the catalog's worked example. The backend root is the
// executable composition layer; api and lifecycle remain independently owned
// importable packages within the generated host module.
package main

import (
	appAPI "futrx.local/catalog/applications/hello-remote/backend/api"
	appLifecycle "futrx.local/catalog/applications/hello-remote/backend/lifecycle"
	"github.com/futrx-com/remote.futrx.com/pkg/applications"
	"github.com/futrx-com/remote.futrx.com/pkg/applications/rpc"
)

func main() {
	rpc.ServeWithRuntime(func(runtime applications.Runtime) applications.Backend {
		return appAPI.New(
			appLifecycle.NewGreetings(runtime.Events),
			appLifecycle.NewInspections(runtime.Events),
		)
	})
}
