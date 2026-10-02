package inspection

import (
	"context"
	"encoding/json"

	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
)

// containerLXDInspector translates LXD instance configuration and runtime
// state into the service's provider-neutral inspection model.
type containerLXDInspector struct {
	commands *quickCommandRunner
}

// instanceConfig mirrors the subset of /1.0/instances/<n> we care about.
// ExpandedConfig merges profile-provided keys (e.g. the futrx-workspace
// resource limits) with container-local config; Config is local-only.
type instanceConfig struct {
	Architecture    string                       `json:"architecture"`
	Type            string                       `json:"type"`
	CreatedAt       string                       `json:"created_at"`
	LastUsedAt      string                       `json:"last_used_at"`
	Config          map[string]string            `json:"config"`
	ExpandedConfig  map[string]string            `json:"expanded_config"`
	Devices         map[string]map[string]string `json:"devices"`
	ExpandedDevices map[string]map[string]string `json:"expanded_devices"`
}

// effectiveConfig returns the value a key actually resolves to on the
// instance: container-local config wins, then profile-provided config.
func (c *instanceConfig) effectiveConfig(key string) string {
	if v := c.Config[key]; v != "" {
		return v
	}
	return c.ExpandedConfig[key]
}

// instanceState mirrors /1.0/instances/<n>/state.
type instanceState struct {
	PID       int    `json:"pid"`
	Processes int    `json:"processes"`
	Status    string `json:"status"`
	CPU       struct {
		Usage int64 `json:"usage"` // nanoseconds
	} `json:"cpu"`
	Disk map[string]struct {
		Usage int64 `json:"usage"`
		Total int64 `json:"total"`
	} `json:"disk"`
	Memory struct {
		Usage     int64 `json:"usage"`
		UsagePeak int64 `json:"usage_peak"`
		Total     int64 `json:"total"`
		Swap      int64 `json:"swap_usage"`
	} `json:"memory"`
	Network map[string]struct {
		HWAddr   string `json:"hwaddr"`
		HostName string `json:"host_name"`
		MTU      int    `json:"mtu"`
		State    string `json:"state"`
		Type     string `json:"type"`
		Counters struct {
			BytesReceived int64 `json:"bytes_received"`
			BytesSent     int64 `json:"bytes_sent"`
		} `json:"counters"`
		Addresses []struct {
			Family  string `json:"family"`
			Address string `json:"address"`
			Netmask string `json:"netmask"`
			Scope   string `json:"scope"`
		} `json:"addresses"`
	} `json:"network"`
}

func (i *containerLXDInspector) inspectConfiguration(ctx context.Context, name string, out *serviceproject.ContainerInspect) {
	cfg, err := i.queryInstance(ctx, name)
	if err != nil {
		return
	}
	out.Architecture = cfg.Architecture
	out.Type = cfg.Type
	out.CreatedAt = cfg.CreatedAt
	out.LastUsedAt = cfg.LastUsedAt
	out.BootAutostart = cfg.Config["boot.autostart"] == "true"
	if description := cfg.Config["image.description"]; description != "" {
		out.Image = description
	} else if alias := cfg.Config["image.alias"]; alias != "" {
		out.Image = alias
	}
	disk := ""
	if root := cfg.ExpandedDevices["root"]; root != nil {
		disk = root["size"]
	}
	if cpu, memory := cfg.effectiveConfig("limits.cpu"), cfg.effectiveConfig("limits.memory"); cpu != "" || memory != "" || disk != "" {
		out.Limits = &serviceproject.ContainerLimits{CPU: cpu, Memory: memory, Disk: disk}
	}
	if workspace, ok := cfg.Devices["workspace"]; ok {
		out.Workspace = &serviceproject.WorkspaceInfo{
			HostSource:    workspace["source"],
			ContainerPath: workspace["path"],
		}
	}
}

func (i *containerLXDInspector) inspectRuntime(ctx context.Context, name string, out *serviceproject.ContainerInspect) {
	state, err := i.queryInstanceState(ctx, name)
	if err != nil {
		return
	}
	out.PID = state.PID
	out.Resources = &serviceproject.ResourceInfo{
		Processes:          state.Processes,
		MemoryCurrentBytes: state.Memory.Usage,
		MemoryPeakBytes:    state.Memory.UsagePeak,
		MemoryTotalBytes:   state.Memory.Total,
		SwapCurrentBytes:   state.Memory.Swap,
		CPUUsageSeconds:    state.CPU.Usage / 1_000_000_000,
	}
	if root, ok := state.Disk["root"]; ok {
		out.Resources.DiskUsageKnown = true
		out.Resources.DiskUsageBytes = root.Usage
	}
	for name, network := range state.Network {
		if name == "lo" {
			continue
		}
		addresses := make([]string, 0, len(network.Addresses))
		for _, address := range network.Addresses {
			addresses = append(addresses, address.Address+"/"+address.Netmask)
		}
		out.Network = append(out.Network, serviceproject.NetworkInterface{
			Name:          name,
			State:         network.State,
			Type:          network.Type,
			HostName:      network.HostName,
			MACAddress:    network.HWAddr,
			MTU:           network.MTU,
			Addresses:     addresses,
			BytesReceived: network.Counters.BytesReceived,
			BytesSent:     network.Counters.BytesSent,
		})
	}
}

func (i *containerLXDInspector) queryInstance(ctx context.Context, name string) (*instanceConfig, error) {
	raw, err := i.commands.run(ctx, "query", "/1.0/instances/"+name)
	if err != nil {
		return nil, err
	}
	var config instanceConfig
	if err := json.Unmarshal([]byte(raw), &config); err != nil {
		return nil, err
	}
	return &config, nil
}

func (i *containerLXDInspector) queryInstanceState(ctx context.Context, name string) (*instanceState, error) {
	raw, err := i.commands.run(ctx, "query", "/1.0/instances/"+name+"/state")
	if err != nil {
		return nil, err
	}
	var state instanceState
	if err := json.Unmarshal([]byte(raw), &state); err != nil {
		return nil, err
	}
	return &state, nil
}
