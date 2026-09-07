package kimi

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	agentruntime "github.com/futrx-com/remote.futrx.com/internal/integration/agents/runtime"
)

func (p *Provider) Capabilities(ctx context.Context, req agent.CapabilityRequest) (agent.Capabilities, error) {
	kimiHome := containerKimiHome
	if req.ContainerName == "" {
		kimiHome = hostKimiHome()
	}
	cmd := agentruntime.NewCapabilityCommand(context.WithoutCancel(ctx), req, []string{"HOME=/root", "KIMI_CODE_HOME=" + kimiHome}, "node", bridgeArgs()...)
	transport, err := startServerTransport(ctx, cmd)
	if err != nil {
		caps := fallbackCapabilities()
		caps.Warning = "Kimi capabilities could not be read from the CLI"
		return caps, err
	}
	defer transport.close()
	var catalog struct {
		Items []json.RawMessage `json:"items"`
	}
	if err := transport.api(ctx, "GET", "/api/v1/models", nil, &catalog); err != nil {
		return fallbackCapabilities(), fmt.Errorf("Kimi models: %w", err)
	}
	var defaults struct {
		Model string `json:"default_model"`
	}
	defaultsErr := transport.api(ctx, "GET", "/api/v1/config", nil, &defaults)
	raw, _ := json.Marshal(map[string]any{"models": catalog.Items})
	caps, err := parseProviderCatalog(raw, "Default model: "+defaults.Model)
	if err != nil {
		return fallbackCapabilities(), err
	}
	if defaultsErr != nil {
		caps.Warning = "Kimi's default model could not be read"
	}
	return caps, nil
}

func fallbackCapabilities() agent.Capabilities {
	return agent.Capabilities{
		Provider:         agent.ProviderKimi,
		Label:            "Kimi",
		Source:           agent.CapabilitySourceFallback,
		Models:           agent.WithAutoModel(nil, "Kimi default"),
		Modes:            agent.ProviderModes(true),
		DefaultMode:      agent.RunModeDefault,
		ApprovalPolicies: kimiApprovalPolicies(),
	}
}

func kimiApprovalPolicies() []agent.CapabilityOption {
	return []agent.CapabilityOption{{Value: "untrusted", Label: "Manual"}, {Value: "on-request", Label: "Ask when needed"}, {Value: "never", Label: "Never ask"}}
}
