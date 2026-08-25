// Package applications is the policy layer for installable "apps" (images):
// databases and other services a user installs with one click, either
// globally (its own dedicated container, shared by the whole server) or
// scoped to a single project (installed inside that project's container).
//
// The catalog of installable images lives under
// integration/containers/applications/images and is loaded through the
// Registry port. Each installed copy is an Instance, persisted through Store
// and realized in a container through Installer.
package applications

// Scope selects where an application runs.
type Scope string

const (
	// ScopeGlobal runs the app in its own dedicated LXD container, reachable
	// by the whole server.
	ScopeGlobal Scope = "global"
	// ScopeProject installs the app inside a single project's container.
	ScopeProject Scope = "project"
)

// Valid reports whether s is a known scope.
func (s Scope) Valid() bool { return s == ScopeGlobal || s == ScopeProject }

// Protocol is the transport a proxy device forwards.
type Protocol string

const (
	ProtocolTCP Protocol = "tcp"
	ProtocolUDP Protocol = "udp"
)

// Port describes how an image exposes itself.
type Port struct {
	// Internal is the port the software listens on inside the container.
	Internal int `json:"internal"`
	// DefaultExternal is the preferred host port; the allocator falls back to
	// the next free port when it is taken.
	DefaultExternal int      `json:"defaultExternal"`
	Protocol        Protocol `json:"protocol"`
	// BindAddress is the host interface the proxy device listens on. Defaults
	// to 127.0.0.1 so databases are not published to the public internet.
	BindAddress string `json:"bindAddress,omitempty"`
}

// EnvVar is a configurable input an image accepts at install time.
type EnvVar struct {
	Key      string `json:"key"`
	Label    string `json:"label,omitempty"`
	Required bool   `json:"required,omitempty"`
	// Secret marks values (passwords) that must never be echoed back to the UI.
	Secret bool `json:"secret,omitempty"`
	// Default is applied when the user leaves the field blank.
	Default string `json:"default,omitempty"`
	// Generate names a generator ("password") used to fill a blank value.
	Generate string `json:"generate,omitempty"`
}

// Healthcheck is an in-container command that reports readiness.
type Healthcheck struct {
	Command string `json:"command,omitempty"`
}

// Connection maps an image's env vars to the canonical fields a client needs
// (user, password, database), so every server surfaces a uniform connection
// panel regardless of how it names its variables.
type Connection struct {
	// User is a static username when the image has no configurable one
	// (e.g. MySQL's "root"). UserEnv takes precedence when set.
	User string `json:"user,omitempty"`
	// UserEnv is the env var holding the username (e.g. POSTGRES_USER).
	UserEnv string `json:"userEnv,omitempty"`
	// PasswordEnv is the env var holding the password.
	PasswordEnv string `json:"passwordEnv,omitempty"`
	// DatabaseEnv is the env var holding the default database, if any.
	DatabaseEnv string `json:"databaseEnv,omitempty"`
}

// Image is one catalog entry loaded from images/<id>/image.json.
type Image struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Category    string   `json:"category,omitempty"`
	Version     string   `json:"version,omitempty"`
	Icon        string   `json:"icon,omitempty"`
	Scopes      []Scope  `json:"scopes"`
	Port        Port     `json:"port"`
	Env         []EnvVar `json:"env,omitempty"`
	// Service is the systemd unit name inside the container used for
	// start/stop/status.
	Service string `json:"service,omitempty"`
	// Install is the install-script filename relative to the image directory.
	Install     string      `json:"install"`
	Healthcheck Healthcheck `json:"healthcheck,omitempty"`
	// Connection maps env vars to canonical user/password/database fields.
	Connection Connection `json:"connection,omitempty"`
	// Base is the LXD image alias used when this app runs as a dedicated
	// (global) container. Empty defaults to the platform default.
	Base string `json:"base,omitempty"`
}

// SupportsScope reports whether the image may be installed at the given scope.
func (im Image) SupportsScope(s Scope) bool {
	for _, sc := range im.Scopes {
		if sc == s {
			return true
		}
	}
	return false
}

// InstanceStatus is the coarse lifecycle state of an installed app.
type InstanceStatus string

const (
	StatusInstalling InstanceStatus = "installing"
	StatusRunning    InstanceStatus = "running"
	StatusStopped    InstanceStatus = "stopped"
	StatusError      InstanceStatus = "error"
)

// Instance is one installed copy of an image.
type Instance struct {
	ID      string `json:"id"`
	ImageID string `json:"imageId"`
	Name    string `json:"name"`
	Scope   Scope  `json:"scope"`
	// ProjectID is set only for ScopeProject instances.
	ProjectID string `json:"projectId,omitempty"`
	// ContainerName is the LXD container the app runs in: a dedicated
	// futrx-app-* container for global scope, or the project's container.
	ContainerName string `json:"containerName"`
	// DeviceName is the LXD proxy device that exposes the app on the host.
	DeviceName   string   `json:"deviceName"`
	InternalPort int      `json:"internalPort"`
	ExternalPort int      `json:"externalPort"`
	BindAddress  string   `json:"bindAddress"`
	Protocol     Protocol `json:"protocol"`
	// Env holds the resolved install inputs (including generated secrets).
	Env       map[string]string `json:"env,omitempty"`
	Status    InstanceStatus    `json:"status"`
	Error     string            `json:"error,omitempty"`
	CreatedAt int64             `json:"createdAt"`
	UpdatedAt int64             `json:"updatedAt"`
}

// View is the API-safe projection of an Instance: secret env values are
// redacted, non-secret ones are kept.
type View struct {
	Instance
	// EnvPublic contains only non-secret env values, keyed by var name.
	EnvPublic map[string]string `json:"envPublic,omitempty"`
}

// Credentials is the full connection detail for an installed instance,
// including secret env values (e.g. the generated database password). It is
// only returned through endpoints that have authorized the caller.
type Credentials struct {
	ContainerName string `json:"containerName"`
	// LXDHost is the bridge DNS name other containers connect to, at
	// InternalPort: "<containerName>.lxd".
	LXDHost      string `json:"lxdHost"`
	InternalPort int    `json:"internalPort"`
	ExternalPort int    `json:"externalPort"`
	BindAddress  string `json:"bindAddress"`
	// Canonical fields resolved from the image's Connection descriptor, so the
	// UI can show a uniform user/password/database for every server.
	Username string            `json:"username,omitempty"`
	Password string            `json:"password,omitempty"`
	Database string            `json:"database,omitempty"`
	Env      map[string]string `json:"env,omitempty"`
}
