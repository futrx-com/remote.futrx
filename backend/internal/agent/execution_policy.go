package agent

import (
	"strings"

	configconstants "github.com/futrx-com/remote.futrx.com/internal/config/constants"
)

// NormalizeApprovalPolicy trims a supported policy and otherwise returns the
// application default.
func NormalizeApprovalPolicy(policy string) string {
	policy, valid := normalizedApprovalPolicy(policy)
	if !valid {
		return configconstants.DefaultAgentApprovalPolicy
	}
	return policy
}

// ValidApprovalPolicy reports whether policy is supported after trimming.
func ValidApprovalPolicy(policy string) bool {
	_, valid := normalizedApprovalPolicy(policy)
	return valid
}

func normalizedApprovalPolicy(policy string) (string, bool) {
	policy = strings.TrimSpace(policy)
	switch policy {
	case "never", "untrusted", "on-request":
		return policy, true
	default:
		return "", false
	}
}

// NormalizeSandboxPolicy trims a supported policy and otherwise returns the
// application default.
func NormalizeSandboxPolicy(policy string) string {
	policy, valid := normalizedSandboxPolicy(policy)
	if !valid {
		return configconstants.DefaultAgentSandboxPolicy
	}
	return policy
}

// ValidSandboxPolicy reports whether policy is supported after trimming.
func ValidSandboxPolicy(policy string) bool {
	_, valid := normalizedSandboxPolicy(policy)
	return valid
}

func normalizedSandboxPolicy(policy string) (string, bool) {
	policy = strings.TrimSpace(policy)
	switch policy {
	case "readOnly", "workspaceWrite", "dangerFullAccess":
		return policy, true
	default:
		return "", false
	}
}
