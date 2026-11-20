package constants

const (
	// DefaultHarnessApprovalPolicy asks only when an operation requires escalation.
	DefaultHarnessApprovalPolicy = "on-request"
	// DefaultHarnessSandboxPolicy permits writes within the active workspace.
	DefaultHarnessSandboxPolicy = "workspaceWrite"
)
