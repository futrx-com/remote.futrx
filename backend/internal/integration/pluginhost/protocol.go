package pluginhost

import "github.com/futrx-com/remote.futrx.com/pkg/appplugin/pluginrpc"

// The host and every plugin agree on one handshake and one plugin name. They
// are aliased here so the rest of this package reads without the SDK's
// package qualifier, and so the single place that would change if the
// transport moved to gRPC is visible.
var pluginHandshake = pluginrpc.Handshake

const backendPluginName = pluginrpc.BackendPluginName

// backendPlugin is the host side of the plugin: it dispenses a client and
// never a server, since the host never implements a backend.
type backendPlugin = pluginrpc.Plugin
