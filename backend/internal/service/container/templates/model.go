// Package templates owns the project stack presets ("templates") a project
// container is created from.
//
// A template is deliberately a LAYER on top of the single shared base image
// rather than its own full image: the target deployment is one small server,
// so baking N complete images is expensive in build time and disk. A template
// therefore is:
//
//  1. the shared base image (futrx-remote-dev-base), plus
//  2. a post-provision script executed once inside the new container, plus
//  3. optional seed files written into the durable /workspace mount, plus
//  4. an optional agent-instructions snippet seeded as /workspace/AGENTS.md.
//
// A template MAY additionally declare a dedicated pre-built image alias
// (futrx-remote-<name>-base). When that alias exists in the container runtime
// the lifecycle launches from it directly (fast path); otherwise it falls back
// to base image + provision script (slow path). Both paths converge on the
// same rootfs contents, and both leave the same marker file behind.
package templates

import "strings"

// DefaultName is the template used by projects created before templates
// existed and by projects that do not request one. It installs nothing, which
// is exactly the pre-template behaviour.
const DefaultName = "blank"

// Container paths owned by template provisioning. They live in the disposable
// rootfs on purpose: replacing a container (workspace upgrade) must re-run the
// provisioning, because everything the script installed outside /workspace is
// gone with the old rootfs.
const (
	// LogPath collects every provisioning run's output inside the container.
	LogPath = "/var/log/remote-template.log"
	// MarkerPath exists only after a provisioning run completed successfully.
	// Its presence makes every later run a no-op.
	MarkerPath = "/var/lib/remote-template.done"
	// FailurePath exists while a provisioning run is in flight and is removed
	// on success, so a backend restart can still tell "failed" from "pending".
	FailurePath = "/var/lib/remote-template.failed"
	// InstructionsPath is where a template's agent-instructions snippet is
	// seeded. Never overwritten when the file already exists.
	InstructionsPath = "/workspace/AGENTS.md"
)

// imageAliasPrefix and imageAliasSuffix bracket the dedicated pre-built image
// alias of a template that declares one.
const (
	imageAliasPrefix = "futrx-remote-"
	imageAliasSuffix = "-base"
)

// Status is the observable state of one container's template provisioning.
type Status string

const (
	// StatusNone means the template has nothing to provision (e.g. "blank").
	StatusNone Status = "none"
	// StatusPending means provisioning is required but has not started (or the
	// backend restarted before it could finish).
	StatusPending Status = "pending"
	// StatusRunning means the provision script is executing.
	StatusRunning Status = "running"
	// StatusDone means the marker file is present.
	StatusDone Status = "done"
	// StatusFailed means the last provisioning attempt exited non-zero.
	StatusFailed Status = "failed"
)

// SeedFile declares one file copied into the container when it is absent.
// Seeding never overwrites: a workspace is durable and may already hold the
// user's own file at that path.
type SeedFile struct {
	// Source is a file name relative to the template's directory.
	Source string `json:"source"`
	// Target is an absolute path inside the container. It must live under
	// /workspace so the seed survives container replacement.
	Target string `json:"target"`
	// Mode is the octal file mode passed to the runtime. Defaults to 644.
	Mode string `json:"mode,omitempty"`
}

// Definition is the on-disk shape of a template's template.json.
type Definition struct {
	Name        string `json:"name"`
	Title       string `json:"title"`
	Description string `json:"description"`
	// Icon is a stable key the frontend maps to a glyph. Unknown keys fall
	// back to a generic icon, so adding a template never breaks the UI.
	Icon string `json:"icon"`
	// ProvisionScript is a file name relative to the template's directory.
	// Empty means the template installs nothing.
	ProvisionScript string `json:"provisionScript,omitempty"`
	// AgentInstructions is a file name relative to the template's directory
	// whose contents are seeded as /workspace/AGENTS.md.
	AgentInstructions string `json:"agentInstructions,omitempty"`
	// SeedFiles are additional files written into /workspace when absent.
	SeedFiles []SeedFile `json:"seedFiles,omitempty"`
	// DefaultPorts are the TCP ports this stack is expected to listen on.
	// Purely informational: previews are discovered, never declared.
	DefaultPorts []int `json:"defaultPorts,omitempty"`
	// PrebuiltImage declares that this template has a dedicated image alias
	// that `build-template-image` can publish.
	PrebuiltImage bool `json:"prebuiltImage,omitempty"`
}

// Template is a validated definition with its referenced files resolved.
type Template struct {
	Definition
	// Script is the provision program's payload (without the shared harness).
	Script []byte
	// Instructions is the agent-instructions snippet, if any.
	Instructions []byte
	// Seeds carries the resolved contents of Definition.SeedFiles.
	Seeds []Seed
}

// Seed is a SeedFile with its content resolved from the embedded files.
type Seed struct {
	Target  string
	Mode    string
	Content []byte
}

// ImageAlias returns the dedicated pre-built image alias for a template, or an
// empty string when the template does not declare one.
func (t Template) ImageAlias() string {
	if !t.PrebuiltImage {
		return ""
	}
	return ImageAlias(t.Name)
}

// ImageAlias returns the conventional dedicated image alias for a template
// name. Exported so the build CLI can name its publish target without loading
// the catalog twice.
func ImageAlias(name string) string {
	return imageAliasPrefix + name + imageAliasSuffix
}

// Provisions reports whether this template has any post-launch work at all.
// A template without work never issues a single runtime call.
func (t Template) Provisions() bool {
	return len(t.Script) > 0 || len(t.Seeds) > 0 || len(t.Instructions) > 0
}

// allSeeds returns the file seeds plus the implicit agent-instructions seed,
// in the order they are written.
func (t Template) allSeeds() []Seed {
	seeds := make([]Seed, 0, len(t.Seeds)+1)
	seeds = append(seeds, t.Seeds...)
	if len(t.Instructions) > 0 {
		seeds = append(seeds, Seed{
			Target:  InstructionsPath,
			Mode:    "644",
			Content: t.Instructions,
		})
	}
	return seeds
}

// Descriptor is the catalog entry published over HTTP. It carries the
// presentation metadata plus whether the fast (pre-built image) path is
// currently available on this host.
type Descriptor struct {
	Name                   string `json:"name"`
	Title                  string `json:"title"`
	Description            string `json:"description"`
	Icon                   string `json:"icon"`
	DefaultPorts           []int  `json:"defaultPorts,omitempty"`
	Default                bool   `json:"default"`
	Provisions             bool   `json:"provisions"`
	PrebuiltImage          string `json:"prebuiltImage,omitempty"`
	PrebuiltImageAvailable bool   `json:"prebuiltImageAvailable"`
}

// State is one container's template provisioning state.
type State struct {
	Template   string
	Title      string
	Status     Status
	Error      string
	LogPath    string
	StartedAt  int64
	FinishedAt int64
}

// NormalizeName lowercases and trims a requested template name.
func NormalizeName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}
