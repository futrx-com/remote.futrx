package containerinfo

import (
	"fmt"
	"os"
	"runtime"
	"strings"
)

type Info struct {
	Hostname         string `json:"hostname"`
	OperatingSystem  string `json:"operatingSystem"`
	Kernel           string `json:"kernel"`
	Architecture     string `json:"architecture"`
	CPUCount         int    `json:"cpuCount"`
	MemoryTotalBytes int64  `json:"memoryTotalBytes"`
	UptimeSeconds    int64  `json:"uptimeSeconds"`
}

func Read() (Info, error) {
	hostname, err := os.Hostname()
	if err != nil {
		return Info{}, fmt.Errorf("hostname: %w", err)
	}
	osName, err := readOSName("/etc/os-release")
	if err != nil {
		return Info{}, err
	}
	kernel, err := os.ReadFile("/proc/sys/kernel/osrelease")
	if err != nil {
		return Info{}, fmt.Errorf("kernel: %w", err)
	}
	memory, err := readMemoryTotal("/proc/meminfo")
	if err != nil {
		return Info{}, err
	}
	uptime, err := readUptime("/proc/uptime")
	if err != nil {
		return Info{}, err
	}
	return Info{
		Hostname: hostname, OperatingSystem: osName,
		Kernel: "Linux " + strings.TrimSpace(string(kernel)), Architecture: runtime.GOARCH,
		CPUCount: runtime.NumCPU(), MemoryTotalBytes: memory, UptimeSeconds: uptime,
	}, nil
}
