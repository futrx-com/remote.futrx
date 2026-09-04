package image

import (
	"errors"
	"fmt"
	"strings"

	"github.com/futrx-com/remote.futrx.com/internal/agent/provisioning"
)

// baseImageInstallPreamble is the provider-neutral part of the shell recipe.
// Agent packages contribute the npm packages and binaries appended below.
const baseImageInstallPreamble = `set -e
export DEBIAN_FRONTEND=noninteractive
apt-get update -qq

# Core build / shell / network deps, plus git + ssh + jq for everyday agent
# work. python3-pip pulls in python3 too. Skip wrangler/aws/gcloud/hcloud —
# project-specific; agent installs them on demand and records in /workspace/setup.sh.
apt-get install -y -qq \
    curl ca-certificates gnupg \
    git openssh-client \
    jq build-essential python3-pip

# Node __NODE_MAJOR__ (provides node + npm + npx for agent CLIs and JS tooling).
NODE_MAJOR=__NODE_MAJOR__
curl -fsSL "https://deb.nodesource.com/setup_${NODE_MAJOR}.x" | bash - >/dev/null 2>&1
apt-get install -y -qq nodejs

# Official GitHub CLI repo. Auth comes from $GITHUB_TOKEN at runtime,
# pushed per-project from the Secrets UI.
curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg \
    | dd of=/usr/share/keyrings/githubcli-archive-keyring.gpg 2>/dev/null
chmod go+r /usr/share/keyrings/githubcli-archive-keyring.gpg
echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" \
    > /etc/apt/sources.list.d/github-cli.list
apt-get update -qq
apt-get install -y -qq gh`

// InstallScript generates the provider-neutral base recipe plus every
// configured agent CLI at its configured version. npm-packaged CLIs install in
// one npm invocation; script-installed CLIs contribute their own pinned
// install program.
func InstallScript(profiles []provisioning.Profile) (string, error) {
	plan, err := collectCLIInstallPlan(profiles)
	if err != nil {
		return "", err
	}
	return renderInstallScript(plan), nil
}

type cliInstallPlan struct {
	npmPackages     []string
	installScripts  []string
	binaries        []string
	versionCommands []versionCommand
}

type versionCommand struct {
	binary    string
	arguments []string
}

type cliInstallPlanCollector struct {
	plan                cliInstallPlan
	seenNPMPackages     map[string]struct{}
	seenBinaries        map[string]struct{}
	seenVersionCommands map[string]struct{}
}

func collectCLIInstallPlan(profiles []provisioning.Profile) (cliInstallPlan, error) {
	collector := cliInstallPlanCollector{
		plan: cliInstallPlan{
			npmPackages:     make([]string, 0, len(profiles)),
			installScripts:  make([]string, 0, len(profiles)),
			binaries:        make([]string, 0, len(profiles)),
			versionCommands: make([]versionCommand, 0, len(profiles)),
		},
		seenNPMPackages:     make(map[string]struct{}, len(profiles)),
		seenBinaries:        make(map[string]struct{}, len(profiles)),
		seenVersionCommands: make(map[string]struct{}, len(profiles)),
	}
	for _, profile := range profiles {
		if profile.CLI.Binary == "" {
			return cliInstallPlan{}, fmt.Errorf("agent profile %q has an incomplete CLI definition", profile.ID)
		}
		switch {
		case profile.CLI.InstallMode == provisioning.InstallWithScript:
			if profile.CLI.InstallScript == "" {
				return cliInstallPlan{}, fmt.Errorf("agent profile %q uses script install but has no install script", profile.ID)
			}
			collector.plan.installScripts = append(collector.plan.installScripts, profile.CLI.InstallScript)
		case profile.CLI.PackageName == "":
			return cliInstallPlan{}, fmt.Errorf("agent profile %q has an incomplete CLI definition", profile.ID)
		default:
			collector.addNPMPackage(profile.CLI.NPMPackage())
		}
		collector.addBinary(profile.CLI.Binary)
		if len(profile.CLI.VersionArgs) > 0 {
			collector.addVersionCommand(profile.CLI.Binary, profile.CLI.VersionArgs)
		}
	}
	if len(collector.plan.npmPackages) == 0 && len(collector.plan.installScripts) == 0 {
		return cliInstallPlan{}, errors.New("no agent profiles configured")
	}
	return collector.plan, nil
}

func (c *cliInstallPlanCollector) addNPMPackage(npmPackage string) {
	if _, exists := c.seenNPMPackages[npmPackage]; exists {
		return
	}
	c.seenNPMPackages[npmPackage] = struct{}{}
	c.plan.npmPackages = append(c.plan.npmPackages, npmPackage)
}

func (c *cliInstallPlanCollector) addBinary(binary string) {
	if _, exists := c.seenBinaries[binary]; exists {
		return
	}
	c.seenBinaries[binary] = struct{}{}
	c.plan.binaries = append(c.plan.binaries, binary)
}

func (c *cliInstallPlanCollector) addVersionCommand(binary string, arguments []string) {
	command := versionCommand{binary: binary, arguments: append([]string(nil), arguments...)}
	renderedCommand := command.render()
	if _, exists := c.seenVersionCommands[renderedCommand]; exists {
		return
	}
	c.seenVersionCommands[renderedCommand] = struct{}{}
	c.plan.versionCommands = append(c.plan.versionCommands, command)
}

func (c versionCommand) render() string {
	var rendered strings.Builder
	rendered.WriteString(shellWord(c.binary))
	for _, argument := range c.arguments {
		rendered.WriteByte(' ')
		rendered.WriteString(shellWord(argument))
	}
	return rendered.String()
}

func renderInstallScript(plan cliInstallPlan) string {
	var script strings.Builder
	script.WriteString(strings.ReplaceAll(
		baseImageInstallPreamble, "__NODE_MAJOR__", provisioning.MustPin("NODE_MAJOR")))
	if len(plan.npmPackages) > 0 {
		script.WriteString("\n\n# Agent CLIs.\nnpm install -g ")
		writeShellWords(&script, plan.npmPackages)
		script.WriteString(" --silent 2>&1 | tail -8")
	}
	for _, installer := range plan.installScripts {
		script.WriteString("\n\n# Script-installed agent CLI.\n(\n")
		script.WriteString(installer)
		script.WriteString("\n)")
	}
	script.WriteString("\n\n# Sanity check the full toolchain.\nwhich ")
	writeShellWords(&script, plan.binaries)
	script.WriteString(" git gh jq node npm python3 ssh\n")
	for _, command := range plan.versionCommands {
		script.WriteString(command.render())
		script.WriteByte('\n')
	}
	script.WriteString("node --version\ngh --version | head -1")
	return script.String()
}

func writeShellWords(script *strings.Builder, values []string) {
	for index, value := range values {
		if index > 0 {
			script.WriteByte(' ')
		}
		script.WriteString(shellWord(value))
	}
}

func shellWord(value string) string {
	for _, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') || strings.ContainsRune("_@%+=:,./-", character) {
			continue
		}
		return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
	}
	return value
}

func description(profiles []provisioning.Profile) string {
	labels := make([]string, 0, len(profiles))
	for _, profile := range profiles {
		if profile.CLI.ImageLabel != "" {
			labels = append(labels, profile.CLI.ImageLabel)
		}
	}
	description := "futrx remote dev base: ubuntu 24.04 + node " + provisioning.MustPin("NODE_MAJOR")
	if len(labels) > 0 {
		description += " + " + strings.Join(labels, " + ")
	}
	return description
}
