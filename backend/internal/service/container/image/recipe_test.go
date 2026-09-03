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
	if _, err := InstallScript(nil); err == nil || err.Error() != "no agent profiles configured" {
		t.Fatalf("InstallScript(nil) error = %v", err)
	}

	profiles := []provisioning.Profile{{
		ID:  "alpha",
		CLI: provisioning.CLISpec{Binary: "alpha"},
	}}
	if _, err := InstallScript(profiles); err == nil || err.Error() != `agent profile "alpha" has an incomplete CLI definition` {
		t.Fatalf("InstallScript(incomplete) error = %v", err)
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
