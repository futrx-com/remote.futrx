package minimax

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	"github.com/futrx-com/remote.futrx.com/internal/agent/provisioning"
	configconstants "github.com/futrx-com/remote.futrx.com/internal/config/constants"
	"github.com/futrx-com/remote.futrx.com/internal/integration/agents/codexharness"
	agentruntime "github.com/futrx-com/remote.futrx.com/internal/integration/agents/runtime"
)

var (
	ErrProjectRequired           = errors.New("MiniMax is available in project chats")
	ErrMiniMaxAPIKeyMissing      = errors.New("MiniMax Token Plan subscription key is not configured; add it in Settings → Agent authentication")
	ErrMiniMaxRuntimeUnavailable = errors.New("MiniMax runtime model catalog cannot be provisioned")
)

func (p *Provider) args(req agent.RunRequest) []string {
	return codexharness.AppServerArgs(miniMaxConfigArgs(req.Model), req.EnableBrowser)
}

func miniMaxConfigArgs(model string) []string {
	providerID := string(agent.ProviderMiniMax)
	return []string{
		"-c", `model="` + model + `"`,
		"-c", `model_provider="` + providerID + `"`,
		"-c", `model_context_window=` + strconv.Itoa(miniMaxModelContextWindow(model)),
		"-c", `model_catalog_json="` + configconstants.MiniMaxContainerCatalog + `"`,
		"-c", `model_providers.` + providerID + `.name="` + configconstants.MiniMaxLabel + `"`,
		"-c", `model_providers.` + providerID + `.base_url="` + configconstants.MiniMaxAPIBaseURL + `"`,
		"-c", `model_providers.` + providerID + `.env_key="` + configconstants.MiniMaxAPIKeyEnvironment + `"`,
		"-c", `model_providers.` + providerID + `.wire_api="` + configconstants.MiniMaxWireAPI + `"`,
	}
}

func (p *Provider) buildCmd(
	ctx context.Context,
	req agent.RunRequest,
	args []string,
	apiKey string,
	modelCatalog []byte,
	emit func(agent.Event),
) (*exec.Cmd, error) {
	if req.ProjectID == "" || p.projectPreparer == nil {
		return nil, ErrProjectRequired
	}
	if apiKey == "" {
		return nil, ErrMiniMaxAPIKeyMissing
	}
	project, err := p.projectPreparer.Prepare(ctx, agent.ProjectPreparationRequest{
		ProjectID:           agent.ProjectID(req.ProjectID),
		ConversationID:      req.ConversationID,
		EnableBrowser:       req.EnableBrowser,
		EnableScheduleTools: req.EnableScheduleTools,
	}, emit)
	if err != nil {
		return nil, err
	}
	if p.runtimeAssets == nil {
		return nil, ErrMiniMaxRuntimeUnavailable
	}
	if err := p.runtimeAssets.Ensure(
		ctx,
		project.ContainerName,
		[]provisioning.RuntimeAsset{miniMaxRuntimeCatalogAsset(modelCatalog)},
	); err != nil {
		return nil, fmt.Errorf("publish MiniMax model catalog: %w", err)
	}
	runtimeEnvironment := make(map[string]string, len(req.RuntimeEnv)+1)
	for key, value := range req.RuntimeEnv {
		runtimeEnvironment[key] = value
	}
	runtimeEnvironment[configconstants.MiniMaxAPIKeyEnvironment] = apiKey
	// The app-server process must outlive request cancellation long enough for
	// codexharness.Run to send turn/interrupt and receive the terminal status.
	return agentruntime.BuildContainerCommand(context.WithoutCancel(ctx), agentruntime.ContainerCommandSpec{
		ContainerName: project.ContainerName,
		Secrets:       project.Secrets,
		ExcludedSecrets: []string{
			"HOME", "CODEX_HOME", "OPENAI_API_KEY", configconstants.MiniMaxAPIKeyEnvironment,
		},
		SuffixEnvironment:  []string{"HOME=/root", "CODEX_HOME=" + configconstants.MiniMaxContainerHome, "OPENAI_API_KEY="},
		RuntimeEnvironment: runtimeEnvironment,
		Binary:             p.binary,
		Arguments:          args,
	}), nil
}

func miniMaxModelContextWindow(model string) int {
	if miniMaxSupportsThinkingToggle(model) {
		return configconstants.MiniMaxLargeModelContextWindow
	}
	return configconstants.MiniMaxDefaultModelContextWindow
}
