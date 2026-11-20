package minimax

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"

	configconstants "github.com/futrx-com/remote.futrx.com/internal/config/constants"
	"github.com/futrx-com/remote.futrx.com/internal/integration/harnesses/codexharness"
	harnessruntime "github.com/futrx-com/remote.futrx.com/internal/integration/harnesses/runtime"
	"github.com/futrx-com/remote.futrx.com/internal/service/harness"
	"github.com/futrx-com/remote.futrx.com/internal/service/harness/provisioning"
)

var (
	ErrProjectRequired           = errors.New("MiniMax is available in project chats")
	ErrMiniMaxAPIKeyMissing      = errors.New("MiniMax Token Plan subscription key is not configured; add it in Settings → Agent authentication")
	ErrMiniMaxRuntimeUnavailable = errors.New("MiniMax runtime model catalog cannot be provisioned")
)

func (p *Provider) args(req harness.RunRequest) []string {
	return codexharness.AppServerArgs(miniMaxConfigArgs(req.Model), req.EnableBrowser)
}

func miniMaxConfigArgs(model string) []string {
	providerID := string(harness.ProviderMiniMax)
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
	req harness.RunRequest,
	args []string,
	apiKey string,
	modelCatalog []byte,
	emit func(harness.Event),
) (*exec.Cmd, harness.PreparedProject, error) {
	if req.ProjectID == "" || p.projectPreparer == nil {
		return nil, harness.PreparedProject{}, ErrProjectRequired
	}
	if apiKey == "" {
		return nil, harness.PreparedProject{}, ErrMiniMaxAPIKeyMissing
	}
	project, err := p.projectPreparer.Prepare(ctx, harness.ProjectPreparationRequest{
		ProjectID:      harness.ProjectID(req.ProjectID),
		ConversationID: req.ConversationID,
		EnableBrowser:  req.EnableBrowser,
	}, emit)
	if err != nil {
		return nil, harness.PreparedProject{}, err
	}
	if p.runtimeAssets == nil {
		return nil, harness.PreparedProject{}, ErrMiniMaxRuntimeUnavailable
	}
	if err := p.runtimeAssets.Ensure(
		ctx,
		project.ContainerName,
		[]provisioning.RuntimeAsset{miniMaxRuntimeCatalogAsset(modelCatalog)},
	); err != nil {
		return nil, harness.PreparedProject{}, fmt.Errorf("publish MiniMax model catalog: %w", err)
	}
	runtimeEnvironment := make(map[string]string, len(req.RuntimeEnv)+1)
	for key, value := range req.RuntimeEnv {
		runtimeEnvironment[key] = value
	}
	runtimeEnvironment[configconstants.MiniMaxAPIKeyEnvironment] = apiKey
	// The app-server process must outlive request cancellation long enough for
	// codexharness.Run to send turn/interrupt and receive the terminal status.
	return harnessruntime.BuildContainerCommand(context.WithoutCancel(ctx), harnessruntime.ContainerCommandSpec{
		ContainerName: project.ContainerName,
		Secrets:       project.Secrets,
		ExcludedSecrets: []string{
			"HOME", "CODEX_HOME", "OPENAI_API_KEY", configconstants.MiniMaxAPIKeyEnvironment,
		},
		SuffixEnvironment:  []string{"HOME=/root", "CODEX_HOME=" + configconstants.MiniMaxContainerHome, "OPENAI_API_KEY="},
		RuntimeEnvironment: runtimeEnvironment,
		Binary:             p.binary,
		Arguments:          args,
	}), project, nil
}

func miniMaxModelContextWindow(model string) int {
	if miniMaxSupportsThinkingToggle(model) {
		return configconstants.MiniMaxLargeModelContextWindow
	}
	return configconstants.MiniMaxDefaultModelContextWindow
}
