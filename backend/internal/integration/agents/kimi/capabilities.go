package kimi

import (
	"context"
	"fmt"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	agentruntime "github.com/futrx-com/remote.futrx.com/internal/integration/agents/runtime"
)

func (p *Provider) Capabilities(ctx context.Context, req agent.CapabilityRequest) (agent.Capabilities, error) {
	kimiHome := containerKimiHome
	if req.ContainerName == "" {
		kimiHome = hostKimiHome()
	}
	modelsCmd := agentruntime.NewCapabilityCommand(
		ctx,
		req,
		[]string{"HOME=/root", "KIMI_CODE_HOME=" + kimiHome},
		"kimi",
		"provider",
		"list",
		"--json",
	)
	modelsOutput, modelsErr := modelsCmd.Output()
	defaultsCmd := agentruntime.NewCapabilityCommand(
		ctx,
		req,
		[]string{"HOME=/root", "KIMI_CODE_HOME=" + kimiHome},
		"kimi",
		"provider",
		"list",
	)
	defaultsOutput, defaultsErr := defaultsCmd.Output()
	helpCmd := agentruntime.NewCapabilityCommand(
		ctx,
		req,
		[]string{"HOME=/root", "KIMI_CODE_HOME=" + kimiHome},
		"kimi",
		"--help",
	)
	helpOutput, helpErr := helpCmd.CombinedOutput()

	if modelsErr != nil {
		caps := fallbackCapabilities()
		caps.Warning = "Kimi capabilities could not be read from the CLI"
		return caps, fmt.Errorf("kimi capability discovery: models: %w", modelsErr)
	}
	caps, err := parseProviderCatalog(modelsOutput, string(helpOutput), string(defaultsOutput))
	if err != nil {
		fallback := fallbackCapabilities()
		fallback.Warning = "Kimi returned an unreadable provider catalog"
		return fallback, err
	}
	if helpErr != nil || defaultsErr != nil {
		caps.Warning = "Some Kimi capability defaults could not be read from the CLI"
	}
	return caps, nil
}

func fallbackCapabilities() agent.Capabilities {
	return agent.Capabilities{
		Provider:    agent.ProviderKimi,
		Label:       "Kimi",
		Source:      agent.CapabilitySourceFallback,
		Models:      agent.WithAutoModel(nil, "Kimi default"),
		Modes:       agent.ProviderModes(false),
		DefaultMode: agent.RunModeDefault,
	}
}
