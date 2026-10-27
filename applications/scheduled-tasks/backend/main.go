package main

import (
	"futrx.local/catalog/applications/scheduled-tasks/backend/api"
	"futrx.local/catalog/applications/scheduled-tasks/backend/lifecycle"
	"github.com/futrx-com/remote.futrx.com/pkg/applications"
	"github.com/futrx-com/remote.futrx.com/pkg/applications/rpc"
)

func main() {
	rpc.ServeWithRuntime(func(runtime applications.Runtime) applications.Backend {
		return api.New(lifecycle.NewTasks(runtime.Events), runtime.AgentTurns)
	})
}
