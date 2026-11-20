package codexharness

import (
	"strings"

	"github.com/futrx-com/remote.futrx.com/internal/service/harness"
)

func buildAppServerThreadRequest(req harness.RunRequest) appServerThreadRequest {
	approvalPolicy := harness.NormalizeApprovalPolicy(req.Preferences.ApprovalPolicy)
	sandboxPolicy := harness.NormalizeSandboxPolicy(req.Preferences.SandboxPolicy)
	request := appServerThreadRequest{
		Method: "thread/start",
		Params: appServerThreadParams{
			ApprovalPolicy: approvalPolicy,
			Sandbox:        legacySandboxMode(sandboxPolicy),
			ServiceName:    "remote-futrx",
		},
	}
	if cwd := strings.TrimSpace(req.Cwd); cwd != "" {
		request.Params.Cwd = cwd
	}
	if model := sanitizeModel(req.Model); model != "" {
		request.Params.Model = model
	}
	if tier := serviceTierArg(req.Preferences.ServiceTier); tier != "" {
		request.Params.ServiceTier = tier
	}
	if req.ResumeID == "" {
		return request
	}
	request.Params.ServiceName = ""
	request.Params.ThreadID = req.ResumeID
	if req.Fork {
		request.Method = "thread/fork"
		return request
	}
	request.Method = "thread/resume"
	return request
}

func buildAppServerTurnParams(req harness.RunRequest, threadID, model string) appServerTurnParams {
	mode := harness.RunModeDefault
	if req.Mode == harness.RunModePlan {
		mode = harness.RunModePlan
	}
	effort := reasoningEffortArg(req.Preferences.ReasoningEffort)
	var reasoningEffort *string
	if effort != "" {
		reasoningEffort = &effort
	}
	params := appServerTurnParams{
		ApprovalPolicy: harness.NormalizeApprovalPolicy(req.Preferences.ApprovalPolicy),
		CollaborationMode: appServerCollaborationMode{
			Mode: mode,
			Settings: appServerCollaborationSettings{
				Model:           model,
				ReasoningEffort: reasoningEffort,
			},
		},
		Effort:        effort,
		Input:         []appServerUserInput{{Text: req.Prompt, Type: "text"}},
		Model:         model,
		SandboxPolicy: appServerSandboxPolicy{Type: harness.NormalizeSandboxPolicy(req.Preferences.SandboxPolicy)},
		ThreadID:      threadID,
	}
	if tier := serviceTierArg(req.Preferences.ServiceTier); tier != "" {
		params.ServiceTier = tier
	}
	return params
}

func legacySandboxMode(policy string) string {
	switch harness.NormalizeSandboxPolicy(policy) {
	case "readOnly":
		return "read-only"
	case "dangerFullAccess":
		return "danger-full-access"
	default:
		return "workspace-write"
	}
}

func sanitizeModel(model string) string {
	model = strings.TrimSpace(model)
	if idx := strings.Index(model, "["); idx > 0 {
		model = strings.TrimSpace(model[:idx])
	}
	return model
}

func reasoningEffortArg(effort harness.ReasoningEffort) string {
	return harness.NormalizeCapabilityValue(string(effort))
}

func serviceTierArg(tier harness.ServiceTier) string {
	return harness.NormalizeCapabilityValue(string(tier))
}
