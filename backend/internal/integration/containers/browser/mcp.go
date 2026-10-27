package browser

// Agent Browser MCP provisioning: makes the @playwright/mcp browser tools
// available to in-container agents, attached over CDP to the live Chrome (the
// SAME session the user logs into). This is the tool layer behind the
// `browser` skill — the agent calls browser_navigate / browser_snapshot /
// browser_click / browser_type etc. instead of hand-writing Playwright recipes.
//
// Shared agent preparation invokes this only when the selected module's factory
// opted into MCP/core support and the run selected the browser skill, so the
// tool surface and per-prompt MCP process do not burden ordinary prompts.

import (
	"context"
	"fmt"
	"path"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/integration/containers/assets"
	"github.com/futrx-com/remote.futrx.com/internal/integration/containers/command"
	serviceprofiles "github.com/futrx-com/remote.futrx.com/internal/service/container/profiles"
	"github.com/futrx-com/remote.futrx.com/internal/shared/output"
)

const (
	browserMCPInstallTimeout = 5 * time.Minute
)

// agentBrowserMCPProvisioner owns installation and profile-defined templates
// for the browser tool server. It is independent of the browser GUI runtime.
type agentBrowserMCPProvisioner struct {
	runner    command.Runner
	profiles  serviceprofiles.Source
	publisher *assets.Publisher
}

// EnsureAgentBrowserMCP installs @playwright/mcp (idempotently) and pushes the
// profile-owned MCP templates. Cheap once installed: the npm-presence check
// short-circuits, and templates are only re-pushed when their content changes.
func (a *Adapter) EnsureMCP(ctx context.Context, containerName string) error {
	return a.mcp.ensure(ctx, containerName)
}

func (p *agentBrowserMCPProvisioner) ensure(ctx context.Context, containerName string) error {
	if !p.runner.Available() {
		return command.ErrUnavailable
	}

	_, missing := command.RunWithTimeout(ctx, p.runner, queryTimeout, "exec", containerName, "--", "sh", "-c", "npm ls -g @playwright/mcp >/dev/null 2>&1")
	if missing != nil {
		out, err := command.RunWithTimeout(ctx, p.runner, browserMCPInstallTimeout, "exec", containerName, "--", "sh", "-c", "npm install -g @playwright/mcp 2>&1 | tail -3")
		if err != nil {
			return fmt.Errorf("install @playwright/mcp: %w; output: %s", err, output.TruncateTail(out, 1000))
		}
	}

	for _, profile := range p.profiles.Snapshot() {
		for _, template := range profile.BrowserMCPTemplates {
			directory := template.Directory
			if directory == "" {
				directory = path.Dir(template.Path)
			}
			directoryMode := template.DirectoryMode
			if directoryMode == "" {
				directoryMode = "755"
			}
			out, err := command.RunWithTimeout(ctx, p.runner, queryTimeout, "exec", containerName, "--",
				"install", "-d", "-m", directoryMode, directory)
			if err != nil {
				return fmt.Errorf("mkdir %s: %w; output: %s", directory, err, out)
			}
			mode := template.Mode
			if mode == "" {
				mode = "644"
			}
			if err := p.publisher.Push(ctx, containerName, template.Content,
				template.HashPath, mode, template.Path); err != nil {
				return err
			}
		}
	}
	return nil
}
