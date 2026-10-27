package image

import (
	"strings"
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/agent/provisioning"
)

func TestRecipeUsesConfiguredProfiles(t *testing.T) {
	profiles := []provisioning.Profile{
		{ID: "alpha", CLI: provisioning.CLISpec{ImageLabel: "alpha-cli", Binary: "alpha", VersionArgs: []string{"--version"}, PackageName: "@example/alpha", Version: "1.2.3"}},
		{ID: "beta", CLI: provisioning.CLISpec{ImageLabel: "beta-cli", Binary: "beta", VersionArgs: []string{"version", "--short"}, PackageName: "@example/beta", Version: "4.5.6"}},
	}
	script, err := InstallScript(profiles)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"@example/alpha@1.2.3", "@example/beta@4.5.6", "which alpha beta", "alpha --version", "beta version --short"} {
		if !strings.Contains(script, want) {
			t.Fatalf("base image install script is missing %q", want)
		}
	}
	if got, want := description(profiles), "futrx remote dev base: ubuntu 24.04 + node 22 + alpha-cli + beta-cli"; got != want {
		t.Fatalf("description = %q, want %q", got, want)
	}
}

func TestInstallScriptRejectsMissingProfilesAndIncompleteCLI(t *testing.T) {
	tests := []struct {
		name     string
		profiles []provisioning.Profile
		wantErr  string
	}{
		{name: "no profiles", wantErr: "no agent profiles configured"},
		{
			name: "missing npm package",
			profiles: []provisioning.Profile{{
				ID:  "alpha",
				CLI: provisioning.CLISpec{Binary: "alpha"},
			}},
			wantErr: `agent profile "alpha" has an incomplete CLI definition`,
		},
		{
			name: "missing install script",
			profiles: []provisioning.Profile{{
				ID: "script-agent",
				CLI: provisioning.CLISpec{
					Binary:      "script-agent",
					InstallMode: provisioning.InstallWithScript,
				},
			}},
			wantErr: `agent profile "script-agent" uses script install but has no install script`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			script, err := InstallScript(test.profiles)
			if script != "" {
				t.Fatalf("InstallScript() script = %q, want empty script on error", script)
			}
			if err == nil || err.Error() != test.wantErr {
				t.Fatalf("InstallScript() error = %v, want %q", err, test.wantErr)
			}
		})
	}
}

func TestInstallScriptQuotesProviderOwnedShellArguments(t *testing.T) {
	profiles := []provisioning.Profile{{
		ID: "future-agent",
		CLI: provisioning.CLISpec{
			Name:        "Future Agent",
			Binary:      "future agent",
			VersionArgs: []string{"version; true", "--format=short"},
			PackageName: "@example/future agent",
			Version:     "1.2.3",
		},
	}}
	script, err := InstallScript(profiles)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"npm install -g '@example/future agent@1.2.3'",
		"which 'future agent' git",
		"'future agent' 'version; true' --format=short",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("base image install script is missing quoted argument %q:\n%s", want, script)
		}
	}
}

func TestInstallScriptDeduplicatesSharedHarnessCLI(t *testing.T) {
	sharedCLI := provisioning.CLISpec{
		ImageLabel:  "codex",
		Binary:      "codex",
		VersionArgs: []string{"--version"},
		PackageName: "@openai/codex",
		Version:     "1.2.3",
	}
	script, err := InstallScript([]provisioning.Profile{
		{ID: "codex", CLI: sharedCLI},
		{ID: "minimax", CLI: sharedCLI},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, sharedEntry := range []string{"@openai/codex@1.2.3", "codex --version"} {
		if count := strings.Count(script, sharedEntry); count != 1 {
			t.Fatalf("%q appears %d times in script:\n%s", sharedEntry, count, script)
		}
	}
	if !strings.Contains(script, "which codex git") {
		t.Fatalf("shared binary sanity check was not deduplicated:\n%s", script)
	}
}

func TestInstallScriptPreservesPlanOrderAndExactShellRendering(t *testing.T) {
	alphaCLI := provisioning.CLISpec{
		Binary:      "alpha cli",
		VersionArgs: []string{"version; true", "--format=short"},
		PackageName: "@example/alpha cli",
		Version:     "1.2.3",
	}
	profiles := []provisioning.Profile{
		{ID: "alpha", CLI: alphaCLI},
		{ID: "beta", CLI: provisioning.CLISpec{
			Binary:        "beta",
			VersionArgs:   []string{"--version"},
			InstallMode:   provisioning.InstallWithScript,
			InstallScript: "curl -fsSL https://example.test/beta | bash\ninstall beta",
		}},
		{ID: "alpha-alias", CLI: alphaCLI},
		{ID: "gamma", CLI: provisioning.CLISpec{
			Binary:      "alpha cli",
			VersionArgs: []string{"version"},
			PackageName: "@example/gamma",
		}},
	}

	script, err := InstallScript(profiles)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.ReplaceAll(baseImageInstallPreamble, "__NODE_MAJOR__", provisioning.MustPin("NODE_MAJOR")) +
		"\n\n# Agent CLIs.\nnpm install -g '@example/alpha cli@1.2.3' @example/gamma --silent 2>&1 | tail -8" +
		"\n\n# Script-installed agent CLI.\n(\ncurl -fsSL https://example.test/beta | bash\ninstall beta\n)" +
		"\n\n# Sanity check the full toolchain.\nwhich 'alpha cli' beta git gh jq node npm python3 ssh\n" +
		"'alpha cli' 'version; true' --format=short\n" +
		"beta --version\n" +
		"'alpha cli' version\n" +
		"node --version\ngh --version | head -1"
	if script != want {
		t.Fatalf("InstallScript() =\n%s\n\nwant:\n%s", script, want)
	}
}
