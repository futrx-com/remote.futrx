// Package appplugin is the contract an installable image's Go backend is
// written against.
//
// An image ships a plugin/ directory of Go source. The server compiles it and
// runs it as a separate process, talking to it over hashicorp/go-plugin. The
// plugin implements Backend; the SPA reaches it through
// /api/applications/<instance>/backend/<path>, so a plugin author writes Go
// and gets an HTTP endpoint their ui/ extension can call.
//
// This package holds only the wire types and the interface. It has no
// dependencies outside the standard library, so the service layer can speak
// about backends without importing the RPC machinery — that lives in
// pkg/appplugin/pluginrpc, which is what a plugin's main() calls.
package appplugin

// APIVersion is the version of this contract. A plugin reports the version it
// was built against in its Descriptor, so the host can refuse a mismatch
// instead of failing in an unreadable way at the first call.
const APIVersion = 1

// Route is one endpoint a plugin advertises. Routes are documentation and
// discovery, not enforcement: the host forwards every path under the
// instance's /backend/ prefix and the plugin decides what to do with it.
type Route struct {
	// Method is an HTTP method, or "*" when the route accepts any.
	Method string `json:"method"`
	// Path is relative to the instance's /backend/ prefix, without a leading
	// slash ("health", "kv/*").
	Path        string `json:"path"`
	Description string `json:"description,omitempty"`
}

// Descriptor is what a plugin says about itself when the host connects. It is
// served to the SPA so an extension can discover the routes it may call.
type Descriptor struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
	// APIVersion is the appplugin.APIVersion the plugin was compiled against.
	APIVersion int     `json:"apiVersion"`
	Routes     []Route `json:"routes,omitempty"`
}

// Caller is the signed-in user the host resolved for a request. It is supplied
// by the server, never by the browser, so a plugin may trust it — and must use
// it, because the transport only checks that the caller may reach the plugin
// at all, not what they may ask it to do.
type Caller struct {
	Email   string `json:"email"`
	IsAdmin bool   `json:"isAdmin"`
}

// Instance is the installed copy of the image this plugin process belongs to.
// One process serves one instance, so these values are fixed for its lifetime
// and are handed over once through Backend.Init.
type Instance struct {
	ID      string `json:"id"`
	ImageID string `json:"imageId"`
	// Scope is "global" or "project".
	Scope string `json:"scope"`
	// ProjectID is set only for project-scoped instances.
	ProjectID string `json:"projectId,omitempty"`
	// ContainerName is the LXD container the image's service side runs in,
	// empty for an image that installs nothing in a container.
	ContainerName string `json:"containerName,omitempty"`
	InternalPort  int    `json:"internalPort,omitempty"`
	ExternalPort  int    `json:"externalPort,omitempty"`
	// Env holds the image's resolved install inputs, including generated
	// secrets: a database plugin needs the password its install script used.
	Env map[string]string `json:"env,omitempty"`
	// DataDir is a per-instance directory on the host the plugin owns and may
	// write to. It survives restarts and is removed when the app is
	// uninstalled.
	DataDir string `json:"dataDir,omitempty"`
}

// Request is one call forwarded from the SPA.
type Request struct {
	Method string `json:"method"`
	// Path is relative to the instance's /backend/ prefix, with no leading
	// slash: a request to /api/applications/ab12/backend/kv/greeting arrives
	// as "kv/greeting".
	Path    string              `json:"path"`
	Query   map[string][]string `json:"query,omitempty"`
	Headers map[string][]string `json:"headers,omitempty"`
	Body    []byte              `json:"body,omitempty"`
	Caller  Caller              `json:"caller"`
}

// Response is what the host turns back into an HTTP response. A zero Status is
// sent as 200.
type Response struct {
	Status  int                 `json:"status"`
	Headers map[string][]string `json:"headers,omitempty"`
	Body    []byte              `json:"body,omitempty"`
}

// Backend is what an image's plugin implements. The host calls Describe once
// on connect, Init once before the first request, and Handle per request.
//
// Handle may be called concurrently. The process is killed when the app is
// stopped or uninstalled, so a plugin must not rely on a graceful shutdown for
// anything it cannot afford to lose.
type Backend interface {
	// Describe reports the plugin's identity and routes.
	Describe() (Descriptor, error)
	// Init hands over the instance this process serves. It runs before any
	// Handle call; returning an error fails the app's install or start.
	Init(Instance) error
	// Handle serves one forwarded request.
	Handle(Request) (Response, error)
}
