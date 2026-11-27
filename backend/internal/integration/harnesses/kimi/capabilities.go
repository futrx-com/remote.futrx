package kimi

import (
	"context"
	"fmt"

	harnessruntime "github.com/futrx-com/remote.futrx.com/internal/integration/harnesses/runtime"
	"github.com/futrx-com/remote.futrx.com/internal/service/harness"
)

func (p *Provider) Capabilities(ctx context.Context, req harness.CapabilityRequest) (harness.Capabilities, error) {
	kimiHome := containerKimiHome
	if req.ContainerName == "" {
		kimiHome = hostKimiHome()
	}
	modelsCmd := harnessruntime.NewCapabilityCommand(
		ctx,
		req,
		[]string{"HOME=/root", "KIMI_CODE_HOME=" + kimiHome},
		"kimi",
		"provider",
		"list",
		"--json",
	)
	modelsOutput, modelsErr := modelsCmd.Output()
	defaultsCmd := harnessruntime.NewCapabilityCommand(
		ctx,
		req,
		[]string{"HOME=/root", "KIMI_CODE_HOME=" + kimiHome},
		"kimi",
		"provider",
		"list",
	)
	defaultsOutput, defaultsErr := defaultsCmd.Output()

	if modelsErr != nil {
		caps := fallbackCapabilities()
		caps.Warning = "Kimi capabilities could not be read from the CLI"
		return caps, fmt.Errorf("kimi capability discovery: models: %w", modelsErr)
	}
	caps, err := parseProviderCatalog(modelsOutput, string(defaultsOutput))
	if err != nil {
		fallback := fallbackCapabilities()
		fallback.Warning = "Kimi returned an unreadable provider catalog"
		return fallback, err
	}
	if defaultsErr != nil {
		caps.Warning = "Some Kimi capability defaults could not be read from the CLI"
	}
	return caps, nil
}

func fallbackCapabilities() harness.Capabilities {
	return harness.Capabilities{
		Provider:    harness.ProviderKimi,
		Label:       "Kimi",
		Source:      harness.CapabilitySourceFallback,
		Models:      harness.WithAutoModel(nil, "Kimi default"),
		Modes:       harness.ProviderModes(false),
		DefaultMode: harness.RunModeDefault,
	}
}
