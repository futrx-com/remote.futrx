// Package applications is the policy layer for installable "apps" (applications):
// databases and other services a user installs with one click, either
// globally (its own dedicated container, shared by the whole server) or
// scoped to a single project (installed inside that project's container).
//
// The catalog of installable applications lives in the repository's applications/
// directory, embedded into the binary, and is loaded through the Registry
// port. Each installed copy is an Instance, persisted through Store
// and realized in a container through Installer.
package applications

import "encoding/json"

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

// Port describes how an application exposes itself.
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

// EnvVar is a configurable input an application accepts at install time.
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

// Connection maps an application's env vars to the canonical fields a client needs
// (user, password, database), so every server surfaces a uniform connection
// panel regardless of how it names its variables.
type Connection struct {
	// User is a static username when the application has no configurable one
	// (e.g. MySQL's "root"). UserEnv takes precedence when set.
	User string `json:"user,omitempty"`
	// UserEnv is the env var holding the username (e.g. POSTGRES_USER).
	UserEnv string `json:"userEnv,omitempty"`
	// PasswordEnv is the env var holding the password.
	PasswordEnv string `json:"passwordEnv,omitempty"`
	// DatabaseEnv is the env var holding the default database, if any.
	DatabaseEnv string `json:"databaseEnv,omitempty"`
}

// HostTool is an executable an application needs on the Remote host itself, next to
// the server process, rather than inside a container. Remote ships no tool of
// its own and keeps no package list: the application supplies the download and the
// checksum it must have, so installing Remote never pulls in a dependency only
// one optional application cares about, and a host that installs nothing keeps
// exactly the software it started with.
//
// The download is fetched over HTTPS and rejected unless it hashes to the
// declared SHA-256, so the application — not the network, and not a package mirror —
// decides what ends up on the host.
type HostTool struct {
	// Name is the executable's filename once installed. It is also how a
	// consumer looks the tool up, so it must be a plain name: no slashes.
	Name string `json:"name"`
	// Version is recorded in the install path, so upgrading an application installs
	// beside the old copy instead of overwriting a binary in use.
	Version string `json:"version"`
	// Downloads is keyed by host architecture as Go names it ("amd64",
	// "arm64"). A host whose architecture is absent cannot install the application.
	Downloads map[string]HostToolDownload `json:"downloads"`
	// VersionArgs runs the installed binary to prove it works before the
	// install is reported as successful. Defaults to ["version"].
	VersionArgs []string `json:"versionArgs,omitempty"`
}

// HostToolDownload is one architecture's artifact.
type HostToolDownload struct {
	// URL must be https. It is fetched verbatim; no mirror is substituted.
	URL string `json:"url"`
	// SHA256 is the hex digest of the bytes at URL, before decompression.
	SHA256 string `json:"sha256"`
	// Compression is "", "gzip" or "bzip2" — how the artifact wraps the single
	// executable. Archives holding more than one file are deliberately not
	// supported: one application, one binary, one checksum to read.
	Compression string `json:"compression,omitempty"`
}

// ApplicationSource says where a catalog entry came from. It is decided by the
// registry that loaded the entry and overwrites anything application.json declares,
// so a package cannot describe itself as built in.
type ApplicationSource string

const (
	// SourceBuiltin marks an application compiled into the server binary.
	SourceBuiltin ApplicationSource = "builtin"
	// SourceUploaded marks an application that came from a package an administrator
	// uploaded, and that can therefore be removed again.
	SourceUploaded ApplicationSource = "uploaded"
)

// Application is one catalog entry loaded from applications/<id>/application.json.
type Application struct {
	// HostTools are executables this application needs on the Remote host, each one
	// downloaded and checksum-verified from the application's own declaration.
	HostTools   []HostTool `json:"hostTools,omitempty"`
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description,omitempty"`
	Category    string     `json:"category,omitempty"`
	Version     string     `json:"version,omitempty"`
	// Icon is either a built-in icon key the frontend knows ("database",
	// "cache", …) or a path to an image inside the application's own ui/ directory
	// ("ui/assets/logo.svg"), which lets an application ship its own mark.
	Icon string `json:"icon,omitempty"`
	// Source is filled in by the registry, not by application.json: it says whether
	// this entry is built into the server or came from an uploaded package,
	// which is what tells the UI whether it can be removed.
	Source ApplicationSource `json:"source,omitempty"`
	Scopes []Scope           `json:"scopes"`
	Port   Port              `json:"port"`
	Env    []EnvVar          `json:"env,omitempty"`
	// Service is the systemd unit name inside the container used for
	// start/stop/status.
	Service string `json:"service,omitempty"`
	// Backend is set when the application ships a backend/ directory. Nil means the
	// application has no Go backend and nothing is compiled or run for it.
	Backend *ApplicationBackend `json:"backend,omitempty"`
	// Install is the install-script filename relative to the application directory.
	Install     string      `json:"install"`
	Healthcheck Healthcheck `json:"healthcheck,omitempty"`
	// Connection maps env vars to canonical user/password/database fields.
	Connection Connection `json:"connection,omitempty"`
	// Base is the LXD image alias used when this app runs as a dedicated
	// (global) container. Empty defaults to the platform default.
	Base string `json:"base,omitempty"`
	// Skills names the agent skills this application ships. It is filled in by the
	// registry from the application's own skills/ directory rather than being
	// declared in application.json: each subdirectory holding a SKILL.md is one
	// skill, published into the project workspace when the application is installed
	// and taken back when it is uninstalled.
	Skills []string `json:"skills,omitempty"`
}

// NeedsContainer reports whether this application has infrastructure to provision.
func (im Application) NeedsContainer() bool { return im.Install != "" }

// NeedsPort reports whether this application exposes its provisioned component.
func (im Application) NeedsPort() bool { return im.NeedsContainer() && im.Port.Internal > 0 }

// MarshalJSON writes the capabilities inferred from the application layout so
// clients do not have to duplicate the inference rules.
func (im Application) MarshalJSON() ([]byte, error) {
	type wire Application // sheds this method, so encoding does not recurse
	return json.Marshal(struct {
		wire
		NeedsContainer bool `json:"needsContainer"`
		NeedsPort      bool `json:"needsPort"`
	}{
		wire:           wire(im),
		NeedsContainer: im.NeedsContainer(),
		NeedsPort:      im.NeedsPort(),
	})
}

// SupportsScope reports whether the application may be installed at the given scope.
func (im Application) SupportsScope(s Scope) bool {
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

// Instance is one installed copy of an application.
type Instance struct {
	ID            string `json:"id"`
	ApplicationID string `json:"applicationId"`
	// ApplicationVersion is the application.json version this copy was last installed
	// from. It is what makes an upgrade detectable: when the catalog's version
	// for the application no longer matches, the install script has to run again.
	// Empty means the instance predates version tracking, which is treated as
	// "unknown, so re-install" — install scripts are idempotent, and assuming
	// the container already holds the new version would be a guess.
	ApplicationVersion string `json:"applicationVersion,omitempty"`
	Name               string `json:"name"`
	Scope              Scope  `json:"scope"`
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
	// Canonical fields resolved from the application's Connection descriptor, so the
	// UI can show a uniform user/password/database for every server.
	Username string            `json:"username,omitempty"`
	Password string            `json:"password,omitempty"`
	Database string            `json:"database,omitempty"`
	Env      map[string]string `json:"env,omitempty"`
}
