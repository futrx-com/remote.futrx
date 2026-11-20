package antigravity

import (
	"context"
	"fmt"

	harnessruntime "github.com/futrx-com/remote.futrx.com/internal/integration/harnesses/runtime"
	"github.com/futrx-com/remote.futrx.com/internal/service/harness"
)

func (p *Provider) Capabilities(ctx context.Context, req harness.CapabilityRequest) (harness.Capabilities, error) {
	environment := []string{"HOME=" + containerAgentHome}

	modelsCmd := harnessruntime.NewCapabilityCommand(ctx, req, environment, "agy", "models")
	modelsOutput, modelsErr := modelsCmd.CombinedOutput()
	helpCmd := harnessruntime.NewCapabilityCommand(ctx, req, environment, "agy", "--help")
	helpOutput, helpErr := helpCmd.CombinedOutput()

	if modelsErr != nil && helpErr != nil {
		caps := fallbackCapabilities()
		caps.Warning = "Antigravity capabilities could not be read from the CLI"
		return caps, fmt.Errorf("antigravity capability discovery: models: %v; help: %w", modelsErr, helpErr)
	}
	caps := parseCLIOutputCatalog(string(modelsOutput), string(helpOutput))
	if modelsErr != nil {
		caps.Source = harness.CapabilitySourceFallback
		caps.Warning = "Sign in to Antigravity in this project to load its model catalog"
		caps.UnavailableReason = "Antigravity is not signed in on the host."
		if req.ContainerName != "" {
			caps.UnavailableReason = "Sign in to Antigravity in this project's Terminal application, then refresh models."
		}
		return caps, modelsErr
	}
	return caps, nil
}

func fallbackCapabilities() harness.Capabilities {
	reasoning := []harness.CapabilityOption{harness.AutoOption()}
	for _, effort := range []string{"low", "medium", "high"} {
		reasoning = append(reasoning, harness.CapabilityOption{Value: effort, Label: capabilityLabel(effort)})
	}
	models := harness.WithAutoModel(nil, "Antigravity default")
	models[0].ReasoningEfforts = reasoning
	return harness.Capabilities{
		Provider:    harness.ProviderAntigravity,
		Label:       "Antigravity",
		Source:      harness.CapabilitySourceFallback,
		Models:      models,
		Modes:       harness.ProviderModes(false),
		DefaultMode: harness.RunModeDefault,
	}
}
