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

type Meta struct {
	ID             ID               `json:"id"`
	Name           string           `json:"name"`
	Slug           string           `json:"slug"`
	Cwd            string           `json:"cwd"`
	ContainerName  string           `json:"containerName"`
	Status         Status           `json:"status"`
	ResourceLimits *ContainerLimits `json:"resourceLimits,omitempty"`
	Order          int64            `json:"order,omitempty"`
	ErrorMsg       string           `json:"errorMsg,omitempty"`
	CreatedAt      int64            `json:"createdAt"`
	UpdatedAt      int64            `json:"updatedAt"`
}

type CreateInput struct {
	Name string `json:"name"`
}

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
	Name           string                 `json:"name"`
	State          ContainerState         `json:"state"`
	BootAutostart  bool                   `json:"bootAutostart"`
	Image          string                 `json:"image,omitempty"`
	Type           string                 `json:"type,omitempty"`
	Architecture   string                 `json:"architecture,omitempty"`
	PID            int                    `json:"pid,omitempty"`
	CreatedAt      string                 `json:"createdAt,omitempty"`
	LastUsedAt     string                 `json:"lastUsedAt,omitempty"`
	Workspace      *WorkspaceInfo         `json:"workspace,omitempty"`
	Resources      *ResourceInfo          `json:"resources,omitempty"`
	Network        []NetworkInterface     `json:"network,omitempty"`
	OS             *OSInfo                `json:"os,omitempty"`
	Disks          []DiskUsage            `json:"disks,omitempty"`
	Limits         *ContainerLimits       `json:"limits,omitempty"`
	LimitOverrides *ContainerLimits       `json:"limitOverrides,omitempty"`
	Claude         ClaudeContainerStatus  `json:"claude"`
	Codex          CodexContainerStatus   `json:"codex"`
	Agents         []AgentContainerStatus `json:"agents,omitempty"`
	AuthBundles    []AuthBundleStatus     `json:"authBundles"`
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
// container integrations and exposed by the project inspection API. The
// Claude and Codex fields remain compatibility mirrors for older clients.
type AgentContainerStatus struct {
	ID                    string `json:"id"`
	Label                 string `json:"label"`
	Installed             bool   `json:"installed"`
	Version               string `json:"version,omitempty"`
	InstructionsPath      string `json:"instructionsPath,omitempty"`
	InstructionsInstalled bool   `json:"instructionsInstalled,omitempty"`
	InstructionsInSync    bool   `json:"instructionsInSync,omitempty"`
}

func (i *ContainerInspect) SetAgentStatuses(statuses []AgentContainerStatus) {
	i.Agents = append([]AgentContainerStatus(nil), statuses...)
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
