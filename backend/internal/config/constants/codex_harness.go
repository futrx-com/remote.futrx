package constants

import "time"

const (
	CodexHarnessBinary               = "codex"
	CodexHarnessPackage              = "@openai/codex"
	CodexHarnessVersionPin           = "CODEX_CLI_VERSION"
	CodexHarnessVersionFlag          = "--version"
	CodexHarnessInstallTimeout       = 5 * time.Minute
	CodexHarnessWaitTimeout          = 2 * time.Minute
	CodexHarnessInterruptTimeout     = 10 * time.Second
	CodexHarnessTerminalDrainTimeout = 2 * time.Second
	CodexHarnessAppServer            = "app-server"
	CodexHarnessBrowserCommand       = `mcp_servers.browser.command="npx"`
	CodexHarnessBrowserArgs          = `mcp_servers.browser.args=["@playwright/mcp","--cdp-endpoint","http://127.0.0.1:9222","--caps=vision"]`

	CodexHarnessStdoutInitialBufferSize = 64 * 1024
	CodexHarnessStdoutMaxBufferSize     = 16 * 1024 * 1024
	CodexHarnessStderrInitialBufferSize = 8 * 1024
	CodexHarnessStderrMaxBufferSize     = 1 * 1024 * 1024
	CodexHarnessStderrCaptureLimit      = 64 * 1024
)
