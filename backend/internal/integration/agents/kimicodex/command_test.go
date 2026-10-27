package kimicodex

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

func TestKimiCodexConfigUsesSelectedModelResponsesAndEnvironmentKey(t *testing.T) {
	args := kimiCodexConfigArgs("k3-256k", 262144)
	for _, want := range []string{
		`model="k3-256k"`,
		`model_provider="kimi-codex"`,
		`model_context_window=262144`,
		`model_catalog_json="/root/.kimi-codex/model-catalog.json"`,
		`model_providers.kimi-codex.base_url="https://api.kimi.ai/coding/v1"`,
		`model_providers.kimi-codex.env_key="KIMI_API_KEY"`,
		`model_providers.kimi-codex.wire_api="responses"`,
	} {
		if !slices.Contains(args, want) {
			t.Fatalf("config args are missing %q: %#v", want, args)
		}
	}
	joined := strings.Join(args, " ")
	if strings.Contains(joined, `model="k3"`) || strings.Contains(joined, "experimental_bearer_token") ||
		strings.Contains(joined, "sk-") {
		t.Fatalf("config contains a hardcoded model or credential: %s", joined)
	}
}

func TestProviderRequiresConfiguredKimiCodexAPIKey(t *testing.T) {
	provider := newProvider(
		kimiCodexTestPreparer{project: agent.PreparedProject{ContainerName: "project-container"}},
		kimiCodexTestAPIKeys{},
		kimiCodexTestModels{},
		&kimiCodexTestRuntimeAssets{},
		"codex",
	)
	if _, err := provider.apiKey(); !errors.Is(err, ErrKimiCodexAPIKeyMissing) {
		t.Fatalf("error = %v, want ErrKimiCodexAPIKeyMissing", err)
	}
}

func TestBuildCmdPublishesLiveCatalogAndPreservesManagedSecret(t *testing.T) {
	runtimeAssets := &kimiCodexTestRuntimeAssets{}
	provider := newProvider(
		kimiCodexTestPreparer{
			project: agent.PreparedProject{
				ContainerName: "project-container",
				Secrets: []agent.ProjectSecret{
					{Key: configconstants.KimiCodexAPIKeyEnvironment, Value: "project-key-must-not-pass"},
					{Key: "OPENAI_API_KEY", Value: "must-not-pass"},
					{Key: "HOME", Value: "/workspace/attacker-home"},
					{Key: "CODEX_HOME", Value: "/workspace/attacker-codex-home"},
				},
			},
		},
		kimiCodexTestAPIKeys{key: "managed-key"},
		kimiCodexTestModels{},
		runtimeAssets,
		"codex",
	)
	modelCatalog := []byte(`{"models":[{"slug":"k3-256k"}]}`)
	req := agent.RunRequest{
		ProjectID: "project",
		Model:     "k3-256k",
		RuntimeEnv: map[string]string{
			configconstants.KimiCodexAPIKeyEnvironment: "request-key-must-not-pass",
		},
	}
	cmd, err := provider.buildCmd(
		context.Background(),
		req,
		provider.args(req, 262144),
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
		"CODEX_HOME=/root/.kimi-codex",
		"OPENAI_API_KEY=",
		"KIMI_API_KEY=managed-key",
		`model="k3-256k"`,
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("command is missing %q: %#v", want, cmd.Args)
		}
	}
	if strings.Contains(joined, "OPENAI_API_KEY=must-not-pass") ||
		strings.Contains(joined, "KIMI_API_KEY=project-key-must-not-pass") ||
		strings.Contains(joined, "KIMI_API_KEY=request-key-must-not-pass") ||
		strings.Contains(joined, "/workspace/attacker-home") ||
		strings.Contains(joined, "/workspace/attacker-codex-home") {
		t.Fatalf("untrusted environment escaped into KimiCodex command: %#v", cmd.Args)
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
		kimiCodexTestPreparer{project: agent.PreparedProject{ContainerName: "project-container"}},
		kimiCodexTestAPIKeys{key: "managed-key"},
		kimiCodexTestModels{},
		&kimiCodexTestRuntimeAssets{},
		"codex",
	)
	ctx, cancel := context.WithCancel(context.Background())
	cmd, err := provider.buildCmd(
		ctx,
		agent.RunRequest{ProjectID: "project", Model: "k3"},
		provider.args(agent.RunRequest{Model: "k3"}, 262144),
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
		kimiCodexTestPreparer{project: agent.PreparedProject{ContainerName: "project-container"}},
		kimiCodexTestAPIKeys{key: "managed-key"},
		kimiCodexTestModels{},
		nil,
		"codex",
	)
	_, err := provider.buildCmd(
		context.Background(),
		agent.RunRequest{ProjectID: "project", Model: "k3"},
		provider.args(agent.RunRequest{Model: "k3"}, 262144),
		"managed-key",
		[]byte(`{"models":[]}`),
		nil,
	)
	if !errors.Is(err, ErrKimiCodexRuntimeUnavailable) {
		t.Fatalf("error = %v, want ErrKimiCodexRuntimeUnavailable", err)
	}
}

func TestArgsWireBrowserThroughCodexHarness(t *testing.T) {
	args := (&Provider{}).args(agent.RunRequest{Model: "k3", EnableBrowser: true}, 262144)
	if !slices.Contains(args, `mcp_servers.browser.command="npx"`) {
		t.Fatalf("browser config = %#v", args)
	}
}

type kimiCodexTestAPIKeys struct {
	key string
}

func (s kimiCodexTestAPIKeys) APIKey() (string, bool) {
	return s.key, s.key != ""
}

type kimiCodexTestPreparer struct {
	project agent.PreparedProject
	err     error
}

func (p kimiCodexTestPreparer) Prepare(
	context.Context,
	agent.ProjectPreparationRequest,
	func(agent.Event),
) (agent.PreparedProject, error) {
	return p.project, p.err
}

type kimiCodexTestRuntimeAssets struct {
	container string
	assets    []provisioning.RuntimeAsset
	err       error
}

func (p *kimiCodexTestRuntimeAssets) Ensure(
	_ context.Context,
	container string,
	assets []provisioning.RuntimeAsset,
) error {
	p.container = container
	p.assets = append([]provisioning.RuntimeAsset(nil), assets...)
	return p.err
}
