package resources

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/futrx-com/remote.futrx.com/internal/integration/containers/command"
	serviceresources "github.com/futrx-com/remote.futrx.com/internal/service/resources"
)

var _ serviceresources.Fleet = (*Manager)(nil)

// ApplyDefaults installs the operator's fleet envelope as the managed
// profile's desired state and converges the profile immediately, so a change
// made in Settings reaches running containers without waiting for their next
// launch. LXD applies profile limits live.
func (m *Manager) ApplyDefaults(ctx context.Context, defaults serviceresources.Limits) error {
	m.SetDefaults(Defaults{
		Memory:    strings.TrimSpace(defaults.Memory),
		CPU:       formatCores(defaults.CPU),
		Processes: formatCount(defaults.Processes),
		Disk:      strings.TrimSpace(defaults.Disk),
	})
	return m.ensureProfile(ctx)
}

// fleetInstance mirrors the fields of /1.0/instances?recursion=1 the aggregate
// guard needs. ExpandedConfig already merges the managed profile's limits with
// any container-local override, which is exactly the ceiling LXD enforces.
type fleetInstance struct {
	Name           string            `json:"name"`
	Status         string            `json:"status"`
	Config         map[string]string `json:"config"`
	ExpandedConfig map[string]string `json:"expanded_config"`
}

// RunningInstances lists every LXD instance with the memory ceiling LXD would
// actually enforce on it. One query serves the whole fleet.
func (m *Manager) RunningInstances(ctx context.Context) ([]serviceresources.Instance, error) {
	if !m.runner.Available() {
		return nil, command.ErrUnavailable
	}
	raw, err := command.RunWithTimeout(ctx, m.runner, queryTimeout, "query", "/1.0/instances?recursion=1")
	if err != nil {
		return nil, fmt.Errorf("query instances: %w; output: %s", err, raw)
	}
	var instances []fleetInstance
	if err := json.Unmarshal([]byte(raw), &instances); err != nil {
		return nil, fmt.Errorf("parse instances: %w", err)
	}
	out := make([]serviceresources.Instance, 0, len(instances))
	for _, instance := range instances {
		memory := instance.Config["limits.memory"]
		if memory == "" {
			memory = instance.ExpandedConfig["limits.memory"]
		}
		out = append(out, serviceresources.Instance{
			Name:    instance.Name,
			Running: strings.EqualFold(instance.Status, "Running"),
			Memory:  memory,
		})
	}
	return out, nil
}

// formatCores renders a core count for LXD's `limits.cpu`, which takes whole
// CPUs. A fractional policy value rounds up so a workspace never gets zero.
func formatCores(cpu float64) string {
	if cpu <= 0 {
		return ""
	}
	cores := int(cpu)
	if float64(cores) < cpu {
		cores++
	}
	if cores < 1 {
		cores = 1
	}
	return strconv.Itoa(cores)
}

func formatCount(value int) string {
	if value <= 0 {
		return ""
	}
	return strconv.Itoa(value)
}
