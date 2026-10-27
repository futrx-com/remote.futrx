// Package codexharness owns the shared Codex CLI and app-server mechanics used
// by provider adapters that supply their own identity, configuration, and
// execution environment.
package codexharness

import (
	"github.com/futrx-com/remote.futrx.com/internal/agent/provisioning"
	configconstants "github.com/futrx-com/remote.futrx.com/internal/config/constants"
)

// NewCLISpec returns the shared Codex CLI installation policy used by
// providers backed by the Codex app-server protocol.
func NewCLISpec(name, imageLabel string) provisioning.CLISpec {
	return provisioning.CLISpec{
		Name:               name,
		ImageLabel:         imageLabel,
		Binary:             configconstants.CodexHarnessBinary,
		VersionArgs:        []string{configconstants.CodexHarnessVersionFlag},
		PackageName:        configconstants.CodexHarnessPackage,
		Version:            provisioning.MustCLIVersion(configconstants.CodexHarnessVersionPin),
		CheckVersion:       true,
		VerifyAfterInstall: true,
		ReportVersion:      true,
		InstallMode:        provisioning.InstallWithNPM,
		InstallTimeout:     configconstants.CodexHarnessInstallTimeout,
		WaitTimeout:        configconstants.CodexHarnessWaitTimeout,
	}
}

// AppServerArgs builds the shared app-server and optional Browser MCP
// arguments around provider-specific Codex configuration arguments.
func AppServerArgs(providerConfig []string, enableBrowser bool) []string {
	args := make([]string, 1, 1+len(providerConfig)+4)
	args[0] = configconstants.CodexHarnessAppServer
	args = append(args, providerConfig...)
	if enableBrowser {
		args = append(args,
			"-c", configconstants.CodexHarnessBrowserCommand,
			"-c", configconstants.CodexHarnessBrowserArgs,
		)
	}
	return args
}
