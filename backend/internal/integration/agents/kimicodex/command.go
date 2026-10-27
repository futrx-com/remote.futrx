package kimicodex

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
	ErrProjectRequired             = errors.New("Kimi (Codex) is available in project chats")
	ErrKimiCodexAPIKeyMissing      = errors.New("Kimi Code API key is not configured; add it in Settings → Agents")
	ErrKimiCodexRuntimeUnavailable = errors.New("Kimi (Codex) runtime model catalog cannot be provisioned")
)

func (p *Provider) args(req agent.RunRequest, contextWindow int) []string {
	return codexharness.AppServerArgs(kimiCodexConfigArgs(req.Model, contextWindow), req.EnableBrowser)
}

func kimiCodexConfigArgs(model string, contextWindow int) []string {
	providerID := string(agent.ProviderKimiCodex)
	return []string{
		"-c", `model="` + model + `"`,
		"-c", `web_search="live"`,
		"-c", `model_provider="` + providerID + `"`,
		"-c", `model_context_window=` + strconv.Itoa(contextWindow),
		"-c", `model_catalog_json="` + configconstants.KimiCodexContainerCatalog + `"`,
		"-c", `model_providers.` + providerID + `.name="` + configconstants.KimiCodexLabel + `"`,
		"-c", `model_providers.` + providerID + `.base_url="` + configconstants.KimiCodexAPIBaseURL + `"`,
		"-c", `model_providers.` + providerID + `.env_key="` + configconstants.KimiCodexAPIKeyEnvironment + `"`,
		"-c", `model_providers.` + providerID + `.wire_api="` + configconstants.KimiCodexWireAPI + `"`,
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
		return nil, ErrKimiCodexAPIKeyMissing
	}
	project, err := p.projectPreparer.Prepare(ctx, agent.ProjectPreparationRequest{
		ProjectID:      agent.ProjectID(req.ProjectID),
		ConversationID: req.ConversationID,
		EnableBrowser:  req.EnableBrowser,
	}, emit)
	if err != nil {
		return nil, err
	}
	if p.runtimeAssets == nil {
		return nil, ErrKimiCodexRuntimeUnavailable
	}
	if err := p.runtimeAssets.Ensure(
		ctx,
		project.ContainerName,
		[]provisioning.RuntimeAsset{kimiCodexRuntimeCatalogAsset(modelCatalog)},
	); err != nil {
		return nil, fmt.Errorf("publish Kimi model catalog: %w", err)
	}
	runtimeEnvironment := make(map[string]string, len(req.RuntimeEnv)+1)
	for key, value := range req.RuntimeEnv {
		runtimeEnvironment[key] = value
	}
	runtimeEnvironment[configconstants.KimiCodexAPIKeyEnvironment] = apiKey
	// The app-server process must outlive request cancellation long enough for
	// codexharness.Run to send turn/interrupt and receive the terminal status.
	return agentruntime.BuildContainerCommand(context.WithoutCancel(ctx), agentruntime.ContainerCommandSpec{
		ContainerName: project.ContainerName,
		Secrets:       project.Secrets,
		ExcludedSecrets: []string{
			"HOME", "CODEX_HOME", "OPENAI_API_KEY", configconstants.KimiCodexAPIKeyEnvironment,
		},
		SuffixEnvironment:  []string{"HOME=/root", "CODEX_HOME=" + configconstants.KimiCodexContainerHome, "OPENAI_API_KEY="},
		RuntimeEnvironment: runtimeEnvironment,
		Binary:             p.binary,
		Arguments:          args,
	}), nil
}
