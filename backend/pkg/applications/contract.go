// Package applications is the contract an installable application's Go backend is
// written against.
//
// An application ships a backend/api/ Go entry point and may keep supporting
// host packages beside it under backend/ (for example backend/lifecycle/). The
// server compiles that host module and runs api as a separate process, talking
// to it over hashicorp/go-plugin. The backend implements Backend; the SPA
// reaches it through /api/applications/<instance>/backend/<path>, so a backend
// author writes Go and gets an HTTP endpoint their ui/ extension can call.
//
// This package holds only the wire types and the interface. It has no
// dependencies outside the standard library, so the service layer can speak
// about backends without importing the RPC machinery — that lives in
// pkg/applications/rpc, which is what a backend's main() calls.
package applications

// APIVersion is the version of this contract. A backend reports the version it
// was built against in its Descriptor, so the host can refuse a mismatch
// instead of failing in an unreadable way at the first call.
const APIVersion = 1

// Route is one endpoint a backend advertises. Routes are documentation and
// discovery, not enforcement: the host forwards every path under the
// instance's /backend/ prefix and the backend decides what to do with it.
type Route struct {
	// Method is an HTTP method, or "*" when the route accepts any.
	Method string `json:"method"`
	// Path is relative to the instance's /backend/ prefix, without a leading
	// slash ("health", "kv/*").
	Path        string `json:"path"`
	Description string `json:"description,omitempty"`
}

// Descriptor is the backend description served to the SPA. A backend reports
// the API version and routes; Remote adds manifest-owned identity after the
// connection so an extension can discover one authoritative description.
type Descriptor struct {
	// Name is supplied by Remote from application.json. A backend may leave it
	// empty; any value it reports is replaced by the package name.
	Name string `json:"name"`
	// Version follows the same rule as Name.
	Version string `json:"version,omitempty"`
	// APIVersion is the applications.APIVersion the backend was compiled against.
	APIVersion int     `json:"apiVersion"`
	Routes     []Route `json:"routes,omitempty"`
	// PublishesEvents is derived by Remote from the installed manifest.
	// SubscribesEvents is derived by the RPC server from EventSubscriber. A
	// backend does not set either value itself.
	PublishesEvents  bool `json:"publishesEvents,omitempty"`
	SubscribesEvents bool `json:"subscribesEvents,omitempty"`
}

// Runtime contains capabilities supplied and owned by Remote. It is created
// by rpc.ServeWithRuntime before the backend is constructed. Applications may
// retain these concurrency-safe capabilities and use them from any business
// layer; they do not implement or initialize them.
type Runtime struct {
	Events EventEmitter
}

// Caller is the signed-in user the host resolved for a request. It is supplied
// by the server, never by the browser, so a backend may trust it — and must use
// it, because the transport only checks that the caller may reach the backend
// at all, not what they may ask it to do.
type Caller struct {
	Email   string `json:"email"`
	IsAdmin bool   `json:"isAdmin"`
}

// RequestContext contains trusted context that Remote resolved for a backend
// call. The browser cannot populate this field: the service clears it on an
// ordinary backend route and stamps it only after the corresponding core
// resource has been authorized.
type RequestContext struct {
	Chat *ChatContext `json:"chat,omitempty"`
}

// ChatContext identifies the chat and workspace for a call made through a
// chat-scoped backend URL. WorkspaceRoot is an absolute host path. It is
// authorization context, not a process sandbox: application backends run with
// the same host privileges on both scoped and unscoped calls.
type ChatContext struct {
	ID            string `json:"id"`
	ProjectID     string `json:"projectId,omitempty"`
	WorkspaceRoot string `json:"workspaceRoot"`
}

// Instance is the installed copy of the application this backend process belongs to.
// One process serves one instance, so these values are fixed for its lifetime
// and are handed over once through Backend.Init.
type Instance struct {
	ID                 string `json:"id"`
	ApplicationID      string `json:"applicationId"`
	ApplicationName    string `json:"applicationName"`
	ApplicationVersion string `json:"applicationVersion"`
	// Publishers and Subscriptions are the validated manifest declarations for
	// this application. They cross the process boundary so the host can
	// authorize publications from this exact installed package and the backend
	// can understand the event capabilities with which it was initialized.
	Publishers    []PublisherDeclaration `json:"publishers,omitempty"`
	Subscriptions []Subscription         `json:"subscriptions,omitempty"`
	// Service is the systemd unit declared by application.json, if any.
	Service string `json:"service,omitempty"`
	// Scope is "global" or "project".
	Scope string `json:"scope"`
	// ProjectID is set only for project-scoped instances.
	ProjectID string `json:"projectId,omitempty"`
	// ContainerName is the LXD container the application's service side runs in,
	// empty for an application that installs nothing in a container.
	ContainerName string `json:"containerName,omitempty"`
	InternalPort  int    `json:"internalPort,omitempty"`
	ExternalPort  int    `json:"externalPort,omitempty"`
	// Env holds the application's resolved install inputs, including generated
	// secrets: a database backend needs the password its install script used.
	Env map[string]string `json:"env,omitempty"`
	// DataDir is a per-instance directory on the host the backend owns and may
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
	Context RequestContext      `json:"context,omitempty"`
}

// Response is what the host turns back into an HTTP response. A zero Status is
// sent as 200. Body is the compatibility path for small, buffered responses.
// Large or seekable responses should be created with Stream instead; the
// stream itself is deliberately process-local and is carried over a separate,
// bounded RPC connection rather than encoded into this value.
type Response struct {
	Status  int                 `json:"status"`
	Headers map[string][]string `json:"headers,omitempty"`
	Body    []byte              `json:"body,omitempty"`
	stream  *responseStream
}

// Backend is the required contract for an application's host backend. All three
// methods are mandatory: rpc.Serve accepts a Backend, so an incomplete
// implementation fails to compile. Describe must report APIVersion or the host
// refuses the backend during its handshake.
//
// Handle may be called concurrently. The process is killed when the app is
// stopped or uninstalled, so a backend must not rely on a graceful shutdown for
// anything it cannot afford to lose.
type Backend interface {
	// Describe reports the backend API version and routes. Remote supplies
	// manifest-owned identity to the descriptor clients receive.
	Describe() (Descriptor, error)
	// Init hands over the instance this process serves. It runs before any
	// Handle call; returning an error fails the app's install or start.
	Init(Instance) error
	// Handle serves one forwarded request.
	Handle(Request) (Response, error)
}
