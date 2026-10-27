package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/futrx-com/remote.futrx.com/pkg/applications"
)

const containerInspectionTimeout = 5 * time.Second

type containerFacts struct {
	Hostname         string `json:"hostname"`
	OperatingSystem  string `json:"operatingSystem"`
	Kernel           string `json:"kernel"`
	Architecture     string `json:"architecture"`
	CPUCount         int    `json:"cpuCount"`
	MemoryTotalBytes int64  `json:"memoryTotalBytes"`
	UptimeSeconds    int64  `json:"uptimeSeconds"`
}

type containerInfo struct {
	Name             string `json:"name"`
	Hostname         string `json:"hostname"`
	OperatingSystem  string `json:"operatingSystem"`
	Kernel           string `json:"kernel"`
	Architecture     string `json:"architecture"`
	CPUCount         int    `json:"cpuCount"`
	MemoryTotalBytes int64  `json:"memoryTotalBytes"`
	UptimeSeconds    int64  `json:"uptimeSeconds"`
	Warning          string `json:"warning,omitempty"`
}

// readContainerFacts invokes the inspector built from backend/container/ in the target
// container. The backend runs on the host and crosses only this fixed command.
func readContainerFacts(name string) (containerFacts, error) {
	ctx, cancel := context.WithTimeout(context.Background(), containerInspectionTimeout)
	defer cancel()

	output, err := exec.CommandContext(
		ctx,
		"lxc", "exec", name, "--", "/usr/local/bin/hello-remote-info",
	).CombinedOutput()
	if err != nil {
		if ctx.Err() != nil {
			return containerFacts{}, ctx.Err()
		}
		return containerFacts{}, fmt.Errorf("lxc exec: %w: %s", err, strings.TrimSpace(string(output)))
	}

	var facts containerFacts
	if err := json.Unmarshal(output, &facts); err != nil {
		return containerFacts{}, fmt.Errorf("decode container response: %w", err)
	}
	return facts, nil
}

func (b *api) container(applications.Request) applications.Response {
	b.mu.Lock()
	name := b.instance.ContainerName
	inspect := b.inspectContainer
	b.mu.Unlock()

	if name == "" {
		return applications.JSON(http.StatusConflict, map[string]string{
			"error": "this install has no LXD container",
		})
	}
	facts, err := inspect(name)
	if err != nil {
		return applications.JSON(http.StatusBadGateway, map[string]string{
			"error": fmt.Sprintf("could not inspect the LXD container: %v", err),
		})
	}
	warning := ""
	if err := b.inspections.ContainerInspected(name, facts.Hostname); err != nil {
		warning = fmt.Sprintf("event not published: %v", err)
	}
	return applications.JSON(http.StatusOK, containerInfo{
		Name:             name,
		Hostname:         facts.Hostname,
		OperatingSystem:  facts.OperatingSystem,
		Kernel:           facts.Kernel,
		Architecture:     facts.Architecture,
		CPUCount:         facts.CPUCount,
		MemoryTotalBytes: facts.MemoryTotalBytes,
		UptimeSeconds:    facts.UptimeSeconds,
		Warning:          warning,
	})
}
