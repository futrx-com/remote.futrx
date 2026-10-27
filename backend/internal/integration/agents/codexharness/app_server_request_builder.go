package codexharness

import (
	"strings"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
)

func buildAppServerThreadRequest(req agent.RunRequest) appServerThreadRequest {
	approvalPolicy := agent.NormalizeApprovalPolicy(req.Preferences.ApprovalPolicy)
	sandboxPolicy := agent.NormalizeSandboxPolicy(req.Preferences.SandboxPolicy)
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

func buildAppServerTurnParams(req agent.RunRequest, threadID, model string) appServerTurnParams {
	mode := agent.RunModeDefault
	if req.Mode == agent.RunModePlan {
		mode = agent.RunModePlan
	}
	effort := reasoningEffortArg(req.Preferences.ReasoningEffort)
	var reasoningEffort *string
	if effort != "" {
		reasoningEffort = &effort
	}
	params := appServerTurnParams{
		ApprovalPolicy: agent.NormalizeApprovalPolicy(req.Preferences.ApprovalPolicy),
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
		SandboxPolicy: appServerSandboxPolicy{Type: agent.NormalizeSandboxPolicy(req.Preferences.SandboxPolicy)},
		ThreadID:      threadID,
	}
	if tier := serviceTierArg(req.Preferences.ServiceTier); tier != "" {
		params.ServiceTier = tier
	}
	return params
}

func legacySandboxMode(policy string) string {
	switch agent.NormalizeSandboxPolicy(policy) {
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

func reasoningEffortArg(effort agent.ReasoningEffort) string {
	return agent.NormalizeCapabilityValue(string(effort))
}

func serviceTierArg(tier agent.ServiceTier) string {
	return agent.NormalizeCapabilityValue(string(tier))
}
