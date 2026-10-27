package constants

const (
	// AgentProcessStderrTailBytes bounds diagnostics retained after process exit.
	AgentProcessStderrTailBytes = 64 << 10

	// DefaultAgentApprovalPolicy asks only when an operation requires escalation.
	DefaultAgentApprovalPolicy = "on-request"
	// DefaultAgentSandboxPolicy permits writes within the active workspace.
	DefaultAgentSandboxPolicy = "workspaceWrite"
)
