package applications

import "github.com/futrx-com/remote.futrx.com/pkg/applications/rpc"

// The host and every backend agree on one handshake and one backend name. They
// are aliased here so the rest of this package reads without the SDK's
// package qualifier, and so the single place that would change if the
// transport moved to gRPC is visible.
var backendHandshake = rpc.Handshake

const backendName = rpc.BackendName

// backendAdapter is the host side of the backend: it dispenses a client and
// never a server, since the host never implements a backend.
type backendAdapter = rpc.Adapter
