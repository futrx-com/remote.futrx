package project

type ID string
type Status string
type ContainerState string
type AgentBrowserStatus string

const (
	StatusUnknown      Status = ""
	StatusProvisioning Status = "provisioning"
	StatusRunning      Status = "running"
	StatusStopped      Status = "stopped"
	StatusError        Status = "error"
	StatusMissing      Status = "missing"
)

const (
	ContainerStateRunning ContainerState = "RUNNING"
	ContainerStateStopped ContainerState = "STOPPED"
	ContainerStateFrozen  ContainerState = "FROZEN"
	ContainerStateMissing ContainerState = "MISSING"
	ContainerStateUnknown ContainerState = "UNKNOWN"
)

const (
	AgentBrowserStatusStarting  AgentBrowserStatus = "starting"
	AgentBrowserStatusReady     AgentBrowserStatus = "ready"
	AgentBrowserStatusCoreReady AgentBrowserStatus = "core-ready"
	AgentBrowserStatusError     AgentBrowserStatus = "error"
	AgentBrowserStatusStopped   AgentBrowserStatus = "stopped"
)

// DefaultTemplate is the stack preset assigned to a project that requests
// none, and the value assumed for metadata written before templates existed.
const DefaultTemplate = "blank"

type Meta struct {
	ID            ID     `json:"id"`
	Name          string `json:"name"`
	Slug          string `json:"slug"`
	Cwd           string `json:"cwd"`
	ContainerName string `json:"containerName"`
	Status        Status `json:"status"`
	// Template is the stack preset the container was created from. It is set
	// once at creation and never changes: the provisioning it drives is
	// applied to the container rootfs and to /workspace, and neither can be
	// rolled back to another stack without destroying the project. Recreate
	// the project to change stacks.
	//
	// Empty means "written before templates existed"; read it through
	// TemplateName, which maps that to DefaultTemplate.
	Template       string           `json:"template,omitempty"`
	ResourceLimits *ContainerLimits `json:"resourceLimits,omitempty"`
	Order          int64            `json:"order,omitempty"`
	ErrorMsg       string           `json:"errorMsg,omitempty"`
	CreatedAt      int64            `json:"createdAt"`
	UpdatedAt      int64            `json:"updatedAt"`
}

// TemplateName is the backward-compatible accessor for Meta.Template: project
// metadata written before templates existed has no field at all, and those
// projects behave exactly like the default template.
func (m Meta) TemplateName() string {
	if m.Template == "" {
		return DefaultTemplate
	}
	return m.Template
}

type CreateInput struct {
	Name string `json:"name"`
	// Template selects the stack preset. Empty means the default.
	Template string `json:"template,omitempty"`
}

// UpdateInput deliberately has no Template field: the template is immutable
// after creation (see Meta.Template).
type UpdateInput struct {
	Name *string `json:"name,omitempty"`
}

func ValidID(id ID) bool {
	if len(id) < 4 || len(id) > 32 {
		return false
	}
	for i := 0; i < len(id); i++ {
		c := id[i]
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}

// ContainerInspect is a debugging snapshot of a project's container as seen
// by the container manager. Populated by Service.InspectContainer and
// exposed at GET /api/projects/{id}/container. Fields are best-effort: a
// stopped or missing container leaves dependent sub-structs zero-valued.
type ContainerInspect struct {
	Name           string                `json:"name"`
	State          ContainerState        `json:"state"`
	BootAutostart  bool                  `json:"bootAutostart"`
	Image          string                `json:"image,omitempty"`
	Type           string                `json:"type,omitempty"`
	Architecture   string                `json:"architecture,omitempty"`
	PID            int                   `json:"pid,omitempty"`
	CreatedAt      string                `json:"createdAt,omitempty"`
	LastUsedAt     string                `json:"lastUsedAt,omitempty"`
	Workspace      *WorkspaceInfo        `json:"workspace,omitempty"`
	Resources      *ResourceInfo         `json:"resources,omitempty"`
	Network        []NetworkInterface    `json:"network,omitempty"`
	OS             *OSInfo               `json:"os,omitempty"`
	Disks          []DiskUsage           `json:"disks,omitempty"`
	Limits         *ContainerLimits      `json:"limits,omitempty"`
	LimitOverrides *ContainerLimits      `json:"limitOverrides,omitempty"`
	Claude         ClaudeContainerStatus `json:"claude"`
	Codex          CodexContainerStatus  `json:"codex"`
	AuthBundles    []AuthBundleStatus    `json:"authBundles"`
	// Template reports the project's stack preset and how far its one-time
	// in-container provisioning has got.
	Template *TemplateStatus `json:"template,omitempty"`
}

// TemplateProvisioningNone is the status reported when a project's template
// installs nothing, so its container is ready as soon as it boots.
const TemplateProvisioningNone = "none"

// TemplateStatus is the project-facing view of template provisioning.
// Status is one of "none", "pending", "running", "done", or "failed".
type TemplateStatus struct {
	Name       string `json:"name"`
	Title      string `json:"title,omitempty"`
	Status     string `json:"status"`
	Error      string `json:"error,omitempty"`
	LogPath    string `json:"logPath,omitempty"`
	StartedAt  int64  `json:"startedAt,omitempty"`
	FinishedAt int64  `json:"finishedAt,omitempty"`
}

type WorkspaceInfo struct {
	HostSource    string `json:"hostSource"`
	ContainerPath string `json:"containerPath"`
}

type ResourceInfo struct {
	Processes          int   `json:"processes,omitempty"`
	DiskUsageBytes     int64 `json:"diskUsageBytes,omitempty"`
	MemoryCurrentBytes int64 `json:"memoryCurrentBytes,omitempty"`
	MemoryPeakBytes    int64 `json:"memoryPeakBytes,omitempty"`
	MemoryTotalBytes   int64 `json:"memoryTotalBytes,omitempty"`
	SwapCurrentBytes   int64 `json:"swapCurrentBytes,omitempty"`
	CPUUsageSeconds    int64 `json:"cpuUsageSeconds,omitempty"`
}

type NetworkInterface struct {
	Name          string   `json:"name"`
	State         string   `json:"state,omitempty"`
	Type          string   `json:"type,omitempty"`
	HostName      string   `json:"hostName,omitempty"`
	MACAddress    string   `json:"macAddress,omitempty"`
	MTU           int      `json:"mtu,omitempty"`
	Addresses     []string `json:"addresses,omitempty"`
	BytesReceived int64    `json:"bytesReceived,omitempty"`
	BytesSent     int64    `json:"bytesSent,omitempty"`
}

type OSInfo struct {
	PrettyName string `json:"prettyName,omitempty"`
	Kernel     string `json:"kernel,omitempty"`
	UptimeSec  int64  `json:"uptimeSec,omitempty"`
	CPUCount   int    `json:"cpuCount,omitempty"`
	Hostname   string `json:"hostname,omitempty"`
}

type DiskUsage struct {
	MountPath  string `json:"mountPath"`
	Filesystem string `json:"filesystem,omitempty"`
	TotalBytes int64  `json:"totalBytes,omitempty"`
	UsedBytes  int64  `json:"usedBytes,omitempty"`
	AvailBytes int64  `json:"availBytes,omitempty"`
}

type ContainerLimits struct {
	CPU    string `json:"cpu,omitempty"`
	Memory string `json:"memory,omitempty"`
	Disk   string `json:"disk,omitempty"`
}

type ClaudeContainerStatus struct {
	Installed         bool   `json:"installed"`
	Version           string `json:"version,omitempty"`
	ClaudeMDInstalled bool   `json:"claudeMdInstalled"`
	ClaudeMDInSync    bool   `json:"claudeMdInSync"`
}

type CodexContainerStatus struct {
	Installed bool   `json:"installed"`
	Version   string `json:"version,omitempty"`
}

// AgentContainerStatus is the provider-neutral diagnostic shape returned by
// container integrations. SetAgentStatuses adapts it to the legacy HTTP fields
// without making the integration depend on individual agent names or paths.
type AgentContainerStatus struct {
	ID                    string
	Installed             bool
	Version               string
	InstructionsInstalled bool
	InstructionsInSync    bool
}

func (i *ContainerInspect) SetAgentStatuses(statuses []AgentContainerStatus) {
	for _, status := range statuses {
		switch status.ID {
		case "claude":
			i.Claude = ClaudeContainerStatus{
				Installed:         status.Installed,
				Version:           status.Version,
				ClaudeMDInstalled: status.InstructionsInstalled,
				ClaudeMDInSync:    status.InstructionsInSync,
			}
		case "codex":
			i.Codex = CodexContainerStatus{
				Installed: status.Installed,
				Version:   status.Version,
			}
		}
	}
}

type AuthBundleStatus struct {
	Name  string                 `json:"name"`
	Files []AuthBundleFileStatus `json:"files"`
}

type AuthBundleFileStatus struct {
	HostPath        string `json:"hostPath"`
	ContainerPath   string `json:"containerPath"`
	HostExists      bool   `json:"hostExists"`
	HostMTime       int64  `json:"hostMtime,omitempty"`
	ContainerExists bool   `json:"containerExists"`
	ContainerMTime  int64  `json:"containerMtime,omitempty"`
	HostNewer       bool   `json:"hostNewer"`
	ContainerNewer  bool   `json:"containerNewer"`
}

// ContainerApp describes one externally-reachable TCP listener inside the
// project's container. Surfaced by Service.ListContainerApps and used by
// the frontend's Browser drawer to populate a pick list of running apps.
type ContainerApp struct {
	Port    int    `json:"port"`
	Address string `json:"address,omitempty"`
	Process string `json:"process,omitempty"`
	PID     int    `json:"pid,omitempty"`
}

// AgentBrowserInfo is the externally visible state of the per-project Agent
// Browser stack. Core is the agent-facing headed Chromium/CDP layer; view is
// the human-facing noVNC layer.
type AgentBrowserInfo struct {
	Status       AgentBrowserStatus `json:"status"`
	Core         string             `json:"core,omitempty"`
	View         string             `json:"view,omitempty"`
	ViewerCount  int                `json:"viewerCount,omitempty"`
	UptimeSec    int64              `json:"uptimeSec,omitempty"`
	LastActivity int64              `json:"lastActivity,omitempty"`
	Slug         string             `json:"slug,omitempty"`
	Port         int                `json:"port,omitempty"`
	URL          string             `json:"url,omitempty"`
	Error        string             `json:"error,omitempty"`
}
