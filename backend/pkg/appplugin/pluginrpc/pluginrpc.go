// Package pluginrpc carries an appplugin.Backend across the process boundary.
//
// It is the only part of the plugin SDK that depends on hashicorp/go-plugin,
// which is why it is separate from appplugin itself: the server's service
// layer speaks about backends using appplugin's types alone, while this
// package is imported by the two ends that actually own a socket — a plugin's
// main() and the host that launches it.
//
// The transport is go-plugin's net/rpc mode rather than gRPC. Plugins are Go
// programs compiled from the image catalog, so there is nothing for a
// language-neutral protocol to buy, and net/rpc keeps a plugin's dependencies
// to this SDK and the standard library. Moving to gRPC later is a change to
// this package and a recompile of the catalog, not a change to appplugin.
package pluginrpc

import (
	"net/rpc"

	goplugin "github.com/hashicorp/go-plugin"

	"github.com/futrx-com/remote.futrx.com/pkg/appplugin"
)

// BackendPluginName is the key both ends use to look the backend up in
// go-plugin's plugin set.
const BackendPluginName = "backend"

// Handshake is the mutual check that stops a plugin binary from being run as a
// normal program and stops the host from talking to an unrelated one. Its
// ProtocolVersion moves only when the wire shape below changes; the contract
// version a plugin was compiled against is reported separately in its
// Descriptor.
var Handshake = goplugin.HandshakeConfig{
	ProtocolVersion:  1,
	MagicCookieKey:   "REMOTE_FUTRX_APP_PLUGIN",
	MagicCookieValue: "b0f2b4b6-remote-futrx-application-backend",
}

// Serve runs a backend as a plugin process. It is the whole of a plugin's
// main():
//
//	func main() { pluginrpc.Serve(&myBackend{}) }
//
// It blocks until the host closes the connection.
func Serve(backend appplugin.Backend) {
	goplugin.Serve(&goplugin.ServeConfig{
		HandshakeConfig: Handshake,
		Plugins: goplugin.PluginSet{
			BackendPluginName: &Plugin{Impl: backend},
		},
	})
}

// Plugin adapts appplugin.Backend to go-plugin. The host constructs it with a
// nil Impl (it only ever asks for a client); the plugin process constructs it
// with its implementation.
type Plugin struct {
	Impl appplugin.Backend
}

var _ goplugin.Plugin = (*Plugin)(nil)

func (p *Plugin) Server(*goplugin.MuxBroker) (any, error) {
	return &server{impl: p.Impl}, nil
}

func (p *Plugin) Client(_ *goplugin.MuxBroker, client *rpc.Client) (any, error) {
	return &Client{client: client}, nil
}
