package serverinfo

type Info struct {
	CollectedAt int64       `json:"collectedAt"`
	Host        HostInfo    `json:"host"`
	CPU         CPUInfo     `json:"cpu"`
	Memory      MemoryInfo  `json:"memory"`
	Storage     StorageInfo `json:"storage"`
	Network     NetworkInfo `json:"network"`
	Process     ProcessInfo `json:"process"`
}

type Snapshot struct {
	Host    HostInfo
	CPU     CPUInfo
	Memory  MemoryInfo
	Storage StorageInfo
	Network NetworkInfo
	Process ProcessInfo
}

type HostInfo struct {
	Hostname         string `json:"hostname"`
	OS               string `json:"os"`
	Platform         string `json:"platform,omitempty"`
	Architecture     string `json:"architecture"`
	Kernel           string `json:"kernel,omitempty"`
	UptimeSec        int64  `json:"uptimeSec,omitempty"`
	BootedAt         int64  `json:"bootedAt,omitempty"`
	ServiceUptimeSec int64  `json:"serviceUptimeSec"`
	AppVersion       string `json:"appVersion"`
	GoVersion        string `json:"goVersion"`
	DataPath         string `json:"dataPath"`
	WorkspacePath    string `json:"workspacePath"`
}

type CPUInfo struct {
	LogicalCores  int      `json:"logicalCores"`
	Model         string   `json:"model,omitempty"`
	UsagePercent  *float64 `json:"usagePercent,omitempty"`
	LoadAverage1  *float64 `json:"loadAverage1,omitempty"`
	LoadAverage5  *float64 `json:"loadAverage5,omitempty"`
	LoadAverage15 *float64 `json:"loadAverage15,omitempty"`
}

type MemoryInfo struct {
	TotalBytes     uint64  `json:"totalBytes"`
	UsedBytes      uint64  `json:"usedBytes"`
	AvailableBytes uint64  `json:"availableBytes"`
	FreeBytes      uint64  `json:"freeBytes"`
	CachedBytes    uint64  `json:"cachedBytes"`
	BuffersBytes   uint64  `json:"buffersBytes"`
	UsagePercent   float64 `json:"usagePercent"`
	SwapTotalBytes uint64  `json:"swapTotalBytes"`
	SwapUsedBytes  uint64  `json:"swapUsedBytes"`
	SwapFreeBytes  uint64  `json:"swapFreeBytes"`
}

type StorageInfo struct {
	TotalBytes     uint64         `json:"totalBytes"`
	UsedBytes      uint64         `json:"usedBytes"`
	AvailableBytes uint64         `json:"availableBytes"`
	UsagePercent   float64        `json:"usagePercent"`
	Mounts         []StorageMount `json:"mounts"`
}

type StorageMount struct {
	InodePercent   *float64 `json:"inodePercent,omitempty"`
	Warning        bool     `json:"warning"`
	Device         string   `json:"device,omitempty"`
	MountPath      string   `json:"mountPath"`
	Filesystem     string   `json:"filesystem,omitempty"`
	TotalBytes     uint64   `json:"totalBytes"`
	UsedBytes      uint64   `json:"usedBytes"`
	AvailableBytes uint64   `json:"availableBytes"`
	UsagePercent   float64  `json:"usagePercent"`
}

type NetworkInfo struct {
	ReceivedBytes uint64             `json:"receivedBytes"`
	SentBytes     uint64             `json:"sentBytes"`
	Interfaces    []NetworkInterface `json:"interfaces"`
}

type NetworkInterface struct {
	Name          string   `json:"name"`
	MTU           int      `json:"mtu,omitempty"`
	HardwareAddr  string   `json:"hardwareAddress,omitempty"`
	Addresses     []string `json:"addresses,omitempty"`
	ReceivedBytes uint64   `json:"receivedBytes"`
	SentBytes     uint64   `json:"sentBytes"`
	Loopback      bool     `json:"loopback"`
	Up            bool     `json:"up"`
}

type ProcessInfo struct {
	PID               int    `json:"pid"`
	Goroutines        int    `json:"goroutines"`
	OpenFileHandles   int    `json:"openFileHandles,omitempty"`
	AllocatedBytes    uint64 `json:"allocatedBytes"`
	HeapInUseBytes    uint64 `json:"heapInUseBytes"`
	SystemMemoryBytes uint64 `json:"systemMemoryBytes"`
}
