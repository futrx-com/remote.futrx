//go:build linux

// Terminal's host backend. The project's shells run in the container's
// remote-terminal service; this backend relays the drawer's requests to it
// through one `lxc exec … remote-terminal bridge` process per installation.
package main

import (
	appAPI "futrx.local/catalog/applications/terminal/backend/api"
	"futrx.local/catalog/applications/terminal/backend/containerio"

	"github.com/futrx-com/remote.futrx.com/pkg/applications/rpc"
)

func main() {
	rpc.Serve(appAPI.New(func(container string) appAPI.Relay {
		return containerio.NewBridge(containerio.BridgeCommand(container))
	}))
}
