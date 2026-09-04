package constants

const (
	// DefaultAgentApprovalPolicy asks only when an operation requires escalation.
	DefaultAgentApprovalPolicy = "on-request"
	// DefaultAgentSandboxPolicy permits writes within the active workspace.
	DefaultAgentSandboxPolicy = "workspaceWrite"
)
