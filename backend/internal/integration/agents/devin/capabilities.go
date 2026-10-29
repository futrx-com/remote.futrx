package devin

import (
	"context"
	"fmt"
	"strings"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	agentruntime "github.com/futrx-com/remote.futrx.com/internal/integration/agents/runtime"
)

// Capabilities discovers the Devin model catalog by probing
// `devin models list --format json`. The probe requires authentication; if the
// CLI is not logged in, the probe fails and the fallback capabilities are
// returned with a warning.
func (p *Provider) Capabilities(ctx context.Context, req agent.CapabilityRequest) (agent.Capabilities, error) {
	cmd := agentruntime.NewCapabilityCommand(
		ctx,
		req,
		[]string{"HOME=/root", "XDG_DATA_HOME=/root/.local/share"},
		p.profile.CLI.Binary,
		"models", "list", "--format", "json",
	)
	output, err := cmd.Output()
	if err != nil {
		caps := fallbackCapabilities()
		caps.Warning = "Devin capabilities could not be read from the CLI (is devin logged in?)"
		return caps, fmt.Errorf("devin capability discovery: %w", err)
	}
	caps, parseErr := parseModelCatalog(output)
	if parseErr != nil {
		fallback := fallbackCapabilities()
		fallback.Warning = "Devin returned an unreadable model catalog"
		return fallback, parseErr
	}
	return caps, nil
}

// fallbackCapabilities returns a minimal capability set when the live probe
// fails. Devin supports default, plan, and bypass modes.
func fallbackCapabilities() agent.Capabilities {
	return agent.Capabilities{
		Provider:    agent.ProviderDevin,
		Label:       "Devin",
		Source:      agent.CapabilitySourceFallback,
		Models:      agent.WithAutoModel(nil, "Devin default"),
		Modes:       devinModes(),
		DefaultMode: agent.RunModeDefault,
	}
}

// devinModes returns the mode options Devin advertises: default, plan, and
// bypass. Bypass auto-approves session/request_permission requests inside the
// ACP harness without surfacing them to the UI, mirroring the
// --dangerously-skip-permissions flag used by the CLI print providers.
func devinModes() []agent.CapabilityOption {
	return append(agent.ProviderModes(true), agent.BypassModeOption())
}

// isNotLoggedInError reports whether a capability probe error indicates the
// CLI is not authenticated. This is used to produce a user-friendly warning.
func isNotLoggedInError(err error) bool {
	if err == nil {
		return false
	}
	lower := strings.ToLower(err.Error())
	return strings.Contains(lower, "not logged in") ||
		strings.Contains(lower, "not authenticated") ||
		strings.Contains(lower, "no credentials")
}
