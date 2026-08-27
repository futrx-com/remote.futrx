package runtime

import (
	"context"
	"os"
	"os/exec"
	"strings"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
)

// NewCapabilityCommand constructs a provider probe for the host or selected
// project container. Host probes inherit the process environment with adapter
// overrides. Project probes use lxc exec from /workspace and pass only the
// adapter's explicit environment overrides to that invocation.
func NewCapabilityCommand(
	ctx context.Context,
	req agent.CapabilityRequest,
	environment []string,
	binary string,
	args ...string,
) *exec.Cmd {
	if strings.TrimSpace(req.ContainerName) == "" {
		cmd := exec.CommandContext(ctx, binary, args...)
		cmd.Env = mergeEnvironment(os.Environ(), environment)
		return cmd
	}
	lxcArgs := []string{"exec", "--cwd", "/workspace"}
	for _, entry := range environment {
		lxcArgs = append(lxcArgs, "--env", entry)
	}
	lxcArgs = append(lxcArgs, req.ContainerName, "--", binary)
	lxcArgs = append(lxcArgs, args...)
	return exec.CommandContext(ctx, "lxc", lxcArgs...)
}

func mergeEnvironment(base, overrides []string) []string {
	overridden := make(map[string]struct{}, len(overrides))
	for _, entry := range overrides {
		name, _, found := strings.Cut(entry, "=")
		if found {
			overridden[name] = struct{}{}
		}
	}

	merged := make([]string, 0, len(base)+len(overrides))
	for _, entry := range base {
		name, _, found := strings.Cut(entry, "=")
		if found {
			if _, replaced := overridden[name]; replaced {
				continue
			}
		}
		merged = append(merged, entry)
	}
	return append(merged, overrides...)
}
