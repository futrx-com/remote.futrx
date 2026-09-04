package minimax

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	"github.com/futrx-com/remote.futrx.com/internal/agent/provisioning"
	configconstants "github.com/futrx-com/remote.futrx.com/internal/config/constants"
)

func TestMiniMaxConfigUsesSelectedModelResponsesAndEnvironmentKey(t *testing.T) {
	args := miniMaxConfigArgs("MiniMax-M2.7")
	for _, want := range []string{
		`model="MiniMax-M2.7"`,
		`model_provider="minimax"`,
		`model_context_window=204800`,
		`model_catalog_json="/root/.minimax/model-catalog.json"`,
		`model_providers.minimax.base_url="https://api.minimax.io/v1"`,
		`model_providers.minimax.env_key="MINIMAX_API_KEY"`,
		`model_providers.minimax.wire_api="responses"`,
	} {
		if !slices.Contains(args, want) {
			t.Fatalf("config args are missing %q: %#v", want, args)
		}
	}
	joined := strings.Join(args, " ")
	if strings.Contains(joined, "MiniMax-M3") || strings.Contains(joined, "experimental_bearer_token") ||
		strings.Contains(joined, "sk-") {
		t.Fatalf("config contains a hardcoded model or credential: %s", joined)
	}
}

func TestMiniMaxContextWindowUsesDocumentedFamilyCapabilities(t *testing.T) {
	if got := miniMaxModelContextWindow("MiniMax-M3"); got != 1_000_000 {
		t.Fatalf("M3 context window = %d", got)
	}
	if got := miniMaxModelContextWindow("MiniMax-M2.7"); got != 204_800 {
		t.Fatalf("M2.7 context window = %d", got)
	}
}

func TestProviderRequiresConfiguredMiniMaxAPIKey(t *testing.T) {
	provider := newProvider(
		miniMaxTestPreparer{project: agent.PreparedProject{ContainerName: "project-container"}},
		miniMaxTestAPIKeys{},
		miniMaxTestModels{},
		&miniMaxTestRuntimeAssets{},
		"codex",
	)
	if _, err := provider.apiKey(); !errors.Is(err, ErrMiniMaxAPIKeyMissing) {
		t.Fatalf("error = %v, want ErrMiniMaxAPIKeyMissing", err)
	}
}

func TestBuildCmdPublishesLiveCatalogAndPreservesManagedSecret(t *testing.T) {
	runtimeAssets := &miniMaxTestRuntimeAssets{}
	provider := newProvider(
		miniMaxTestPreparer{
			project: agent.PreparedProject{
				ContainerName: "project-container",
				Secrets: []agent.ProjectSecret{
					{Key: configconstants.MiniMaxAPIKeyEnvironment, Value: "project-key-must-not-pass"},
					{Key: "OPENAI_API_KEY", Value: "must-not-pass"},
					{Key: "HOME", Value: "/workspace/attacker-home"},
					{Key: "CODEX_HOME", Value: "/workspace/attacker-codex-home"},
				},
			},
		},
		miniMaxTestAPIKeys{key: "managed-key"},
		miniMaxTestModels{},
		runtimeAssets,
		"codex",
	)
	modelCatalog := []byte(`{"models":[{"slug":"MiniMax-M2.7"}]}`)
	req := agent.RunRequest{
		ProjectID: "project",
		Model:     "MiniMax-M2.7",
		RuntimeEnv: map[string]string{
			configconstants.MiniMaxAPIKeyEnvironment: "request-key-must-not-pass",
		},
	}
	cmd, err := provider.buildCmd(
		context.Background(),
		req,
		provider.args(req),
		"managed-key",
		modelCatalog,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if runtimeAssets.container != "project-container" || len(runtimeAssets.assets) != 1 ||
		string(runtimeAssets.assets[0].Content) != string(modelCatalog) {
		t.Fatalf("published catalog = (%q, %#v)", runtimeAssets.container, runtimeAssets.assets)
	}
	joined := strings.Join(cmd.Args, " ")
	for _, want := range []string{
		"HOME=/root",
		"CODEX_HOME=/root/.minimax",
		"OPENAI_API_KEY=",
		"MINIMAX_API_KEY=managed-key",
		`model="MiniMax-M2.7"`,
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("command is missing %q: %#v", want, cmd.Args)
		}
	}
	if strings.Contains(joined, "OPENAI_API_KEY=must-not-pass") ||
		strings.Contains(joined, "MINIMAX_API_KEY=project-key-must-not-pass") ||
		strings.Contains(joined, "MINIMAX_API_KEY=request-key-must-not-pass") ||
		strings.Contains(joined, "/workspace/attacker-home") ||
		strings.Contains(joined, "/workspace/attacker-codex-home") {
		t.Fatalf("untrusted environment escaped into MiniMax command: %#v", cmd.Args)
	}
}

func TestBuildCmdProcessOutlivesRequestCancellation(t *testing.T) {
	binDir := t.TempDir()
	lxcPath := filepath.Join(binDir, "lxc")
	if err := os.WriteFile(lxcPath, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	provider := newProvider(
		miniMaxTestPreparer{project: agent.PreparedProject{ContainerName: "project-container"}},
		miniMaxTestAPIKeys{key: "managed-key"},
		miniMaxTestModels{},
		&miniMaxTestRuntimeAssets{},
		"codex",
	)
	ctx, cancel := context.WithCancel(context.Background())
	cmd, err := provider.buildCmd(
		ctx,
		agent.RunRequest{ProjectID: "project", Model: "MiniMax-M3"},
		provider.args(agent.RunRequest{Model: "MiniMax-M3"}),
		"managed-key",
		[]byte(`{"models":[]}`),
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	cancel()

	if err := cmd.Run(); err != nil {
		t.Fatalf("app-server command did not outlive request cancellation: %v", err)
	}
}

func TestBuildCmdRequiresRuntimeCatalogProvisioner(t *testing.T) {
	provider := newProvider(
		miniMaxTestPreparer{project: agent.PreparedProject{ContainerName: "project-container"}},
		miniMaxTestAPIKeys{key: "managed-key"},
		miniMaxTestModels{},
		nil,
		"codex",
	)
	_, err := provider.buildCmd(
		context.Background(),
		agent.RunRequest{ProjectID: "project", Model: "MiniMax-M3"},
		provider.args(agent.RunRequest{Model: "MiniMax-M3"}),
		"managed-key",
		[]byte(`{"models":[]}`),
		nil,
	)
	if !errors.Is(err, ErrMiniMaxRuntimeUnavailable) {
		t.Fatalf("error = %v, want ErrMiniMaxRuntimeUnavailable", err)
	}
}

func TestArgsWireBrowserThroughCodexHarness(t *testing.T) {
	args := (&Provider{}).args(agent.RunRequest{Model: "MiniMax-M3", EnableBrowser: true})
	if !slices.Contains(args, `mcp_servers.browser.command="npx"`) {
		t.Fatalf("browser config = %#v", args)
	}
}

type miniMaxTestAPIKeys struct {
	key string
}

func (s miniMaxTestAPIKeys) APIKey() (string, bool) {
	return s.key, s.key != ""
}

type miniMaxTestPreparer struct {
	project agent.PreparedProject
	err     error
}

func (p miniMaxTestPreparer) Prepare(
	context.Context,
	agent.ProjectPreparationRequest,
	func(agent.Event),
) (agent.PreparedProject, error) {
	return p.project, p.err
}

type miniMaxTestRuntimeAssets struct {
	container string
	assets    []provisioning.RuntimeAsset
	err       error
}

func (p *miniMaxTestRuntimeAssets) Ensure(
	_ context.Context,
	container string,
	assets []provisioning.RuntimeAsset,
) error {
	p.container = container
	p.assets = append([]provisioning.RuntimeAsset(nil), assets...)
	return p.err
}
