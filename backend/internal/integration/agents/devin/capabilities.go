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
// fails. Devin supports default and plan modes.
func fallbackCapabilities() agent.Capabilities {
	return agent.Capabilities{
		Provider:    agent.ProviderDevin,
		Label:       "Devin",
		Source:      agent.CapabilitySourceFallback,
		Models:      agent.WithAutoModel(nil, "Devin default"),
		Modes:       agent.ProviderModes(true),
		DefaultMode: agent.RunModeDefault,
	}
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
