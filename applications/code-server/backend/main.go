package main

import (
	appAPI "futrx.local/catalog/applications/code-server/backend/api"
	"github.com/futrx-com/remote.futrx.com/pkg/applications/rpc"
)

func main() { rpc.Serve(appAPI.New()) }
