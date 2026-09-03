package config

import (
	"slices"
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	"github.com/futrx-com/remote.futrx.com/internal/agent/provisioning"
	agentmodule "github.com/futrx-com/remote.futrx.com/internal/service/agent/module"
)

func TestCatalogBuildsEveryDeclaredAgentInStableOrder(t *testing.T) {
	catalog, err := NewAgentModules()
	if err != nil {
		t.Fatal(err)
	}
	descriptors := catalog.Descriptors()
	profiles := catalog.Profiles()
	ids := make([]agent.ProviderID, len(descriptors))
	for index, descriptor := range descriptors {
		ids[index] = descriptor.ID
		if index >= len(profiles) || profiles[index].ID != string(descriptor.ID) {
			t.Fatalf("descriptor %q has no aligned profile", descriptor.ID)
		}
	}
	want := []agent.ProviderID{
		agent.ProviderClaude,
		agent.ProviderCodex,
		agent.ProviderKimi,
		agent.ProviderAntigravity,
	}
	if !slices.Equal(ids, want) {
		t.Fatalf("agent order = %v, want %v", ids, want)
	}
	if !catalog.HasProvider("claude") || catalog.HasProvider("future-agent") {
		t.Fatal("catalog provider membership is incorrect")
	}
	if got := catalog.DefaultProvider(agentmodule.ScopeHost); got != agent.ProviderCodex {
		t.Fatalf("host default = %q, want codex", got)
	}
	if roots := catalog.LegacySkillRoots("codex"); !slices.Equal(roots, []string{"/root/.codex/skills"}) {
		t.Fatalf("Codex legacy skill roots = %v", roots)
	}
	if !catalog.SupportsNativeFork(string(agent.ProviderClaude)) ||
		!catalog.SupportsNativeFork(string(agent.ProviderCodex)) ||
		catalog.SupportsNativeFork(string(agent.ProviderKimi)) ||
		catalog.SupportsNativeFork(string(agent.ProviderAntigravity)) {
		t.Fatal("catalog native-fork policies do not match provider behavior")
	}
	hostProfiles := catalog.HostProfiles()
	hostIDs := make([]string, len(hostProfiles))
	for index, profile := range hostProfiles {
		hostIDs[index] = profile.ID
	}
	if !slices.Equal(hostIDs, []string{"claude", "codex", "kimi", "antigravity"}) {
		t.Fatalf("host profile order = %v", hostIDs)
	}
	antigravityCLI := hostProfiles[len(hostProfiles)-1].CLI
	if antigravityCLI.Binary != "agy" || antigravityCLI.InstallMode != provisioning.InstallWithScript || antigravityCLI.InstallScript == "" {
		t.Fatalf("Antigravity host CLI policy = %#v", antigravityCLI)
	}

	runtime, err := catalog.Build(agentmodule.BuildDependencies{})
	if err != nil {
		t.Fatal(err)
	}
	for _, descriptor := range descriptors {
		provider := runtime.Lookup(descriptor.ID)
		if provider == nil || provider.ID() != descriptor.ID {
			t.Fatalf("provider %q was not built consistently", descriptor.ID)
		}
		binding, ok := runtime.AuthBinding(descriptor.ID)
		if !ok || binding.ID() != descriptor.ID {
			t.Fatalf("auth binding %q was not built consistently", descriptor.ID)
		}
	}
	if got := runtime.WorkspaceSkillHome("claude"); got != "/workspace/.claude" {
		t.Fatalf("Claude workspace skill home = %q", got)
	}
	if got := runtime.WorkspaceSkillHome("codex"); got != "/workspace/.codex" {
		t.Fatalf("Codex workspace skill home = %q", got)
	}
	if runtime.AnyAuthenticated() != runtime.AccessReady() {
		t.Fatal("built-in access gate drifted from managed auth readiness")
	}
}

func TestCatalogProfilesAreDefensiveCopies(t *testing.T) {
	catalog, err := NewAgentModules()
	if err != nil {
		t.Fatal(err)
	}
	first := catalog.Profiles()
	first[0].Credentials.Files[0].HostPath = "/changed"
	first[0].BrowserMCPTemplates[0].Content[0] = 'x'

	second := catalog.Profiles()
	if second[0].Credentials.Files[0].HostPath == "/changed" {
		t.Fatal("credential policy mutation escaped the catalog")
	}
	if second[0].BrowserMCPTemplates[0].Content[0] == 'x' {
		t.Fatal("template mutation escaped the catalog")
	}

	hostFirst := catalog.HostProfiles()
	hostFirst[0].CLI.Binary = "changed"
	if got := catalog.HostProfiles()[0].CLI.Binary; got == "changed" {
		t.Fatal("host CLI policy mutation escaped the catalog")
	}
}
