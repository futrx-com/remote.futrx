package opencode

import (
	"bufio"
	"context"
	"fmt"
	"strings"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	agentruntime "github.com/futrx-com/remote.futrx.com/internal/integration/agents/runtime"
)

func (p *Provider) Capabilities(ctx context.Context, req agent.CapabilityRequest) (agent.Capabilities, error) {
	modelsCmd := agentruntime.NewCapabilityCommand(
		ctx,
		req,
		[]string{"HOME=/root", "XDG_DATA_HOME=/root/.local/share"},
		"opencode",
		"models",
	)
	output, err := modelsCmd.Output()
	if err != nil {
		caps := fallbackCapabilities()
		caps.Warning = "OpenCode capabilities could not be read from the CLI"
		return caps, fmt.Errorf("opencode capability discovery: models: %w", err)
	}
	caps, parseErr := parseModelsOutput(string(output))
	if parseErr != nil {
		fallback := fallbackCapabilities()
		fallback.Warning = "OpenCode returned an unreadable model list"
		return fallback, parseErr
	}
	return caps, nil
}

func fallbackCapabilities() agent.Capabilities {
	return agent.Capabilities{
		Provider:    agent.ProviderOpenCode,
		Label:       "OpenCode",
		Source:      agent.CapabilitySourceFallback,
		Models:      agent.WithAutoModel(nil, "OpenCode default"),
		Modes:       agent.ProviderModes(true),
		DefaultMode: agent.RunModeDefault,
	}
}

// parseModelsOutput reads `opencode models` text output. Each non-empty line
// is a model id in provider/model form (e.g. "anthropic/claude-sonnet-4-5").
func parseModelsOutput(output string) (agent.Capabilities, error) {
	scanner := bufio.NewScanner(strings.NewReader(output))
	models := make([]agent.ModelCapability, 0, 16)
	for scanner.Scan() {
		id := strings.TrimSpace(scanner.Text())
		if id == "" || strings.ContainsAny(id, "\t\x1b") {
			// Skip TUI decoration / ANSI lines; only bare ids are accepted.
			continue
		}
		if !strings.Contains(id, "/") || strings.Contains(id, " ") {
			continue
		}
		models = append(models, agent.ModelCapability{ID: id, Label: id})
	}
	if len(models) == 0 {
		return agent.Capabilities{}, fmt.Errorf("opencode models output contained no model ids")
	}
	return agent.Capabilities{
		Provider:    agent.ProviderOpenCode,
		Label:       "OpenCode",
		Source:      agent.CapabilitySourceLive,
		Models:      agent.WithAutoModel(models, "OpenCode default"),
		Modes:       agent.ProviderModes(true),
		DefaultMode: agent.RunModeDefault,
	}, nil
}
