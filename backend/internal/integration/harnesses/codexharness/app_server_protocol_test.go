package codexharness

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/service/harness"
)

func TestPlatformInstructionsUseApplicationContextWithoutChangingUserOrModeInstructions(t *testing.T) {
	for _, mode := range []harness.RunMode{harness.RunModeDefault, harness.RunModePlan} {
		for _, session := range []harness.RunRequest{{}, {ResumeID: "existing"}, {ResumeID: "existing", Fork: true}} {
			session.Mode = mode
			session.Prompt = "the user's message"
			session.SystemInstructions = "platform guidance\nwith quotes: \"literal\" and $(literal)"
			params := buildAppServerTurnParams(session, "thread", "model")
			context := params.AdditionalContext["remote-futrx/platform"]
			if context.Kind != "application" || context.Value != session.SystemInstructions {
				t.Fatalf("platform context = %#v", context)
			}
			if len(params.Input) != 1 || params.Input[0].Text != session.Prompt {
				t.Fatalf("platform instructions leaked into user input: %#v", params.Input)
			}
			if params.CollaborationMode.Mode != mode || params.CollaborationMode.Settings.DeveloperInstructions != nil {
				t.Fatalf("built-in mode instructions were replaced: %#v", params.CollaborationMode)
			}
			data, err := json.Marshal(params)
			if err != nil || !strings.Contains(string(data), `"additionalContext"`) {
				t.Fatalf("wire context is missing: %s, %v", data, err)
			}
		}
	}
	data, err := json.Marshal(buildAppServerTurnParams(harness.RunRequest{}, "thread", "model"))
	if err != nil || strings.Contains(string(data), `"additionalContext"`) {
		t.Fatalf("host run received platform context: %s, %v", data, err)
	}
}

func TestAppServerTurnIncludesReasoningEffort(t *testing.T) {
	params := buildAppServerTurnParams(harness.RunRequest{
		Preferences: harness.RunPreferences{ReasoningEffort: "high"},
	}, "thread-1", "gpt-5.5")
	if params.Effort != "high" {
		t.Fatalf("effort = %q, want high", params.Effort)
	}
}

func TestAppServerTurnIgnoresInvalidReasoningEffort(t *testing.T) {
	params := buildAppServerTurnParams(harness.RunRequest{
		Preferences: harness.RunPreferences{ReasoningEffort: "extreme;invalid"},
	}, "thread-1", "gpt-5.5")

	if params.Effort != "" {
		t.Fatalf("invalid reasoning effort should not be sent: %#v", params)
	}
}

func TestAppServerTurnIncludesServiceTier(t *testing.T) {
	params := buildAppServerTurnParams(harness.RunRequest{
		Preferences: harness.RunPreferences{ServiceTier: "priority"},
	}, "thread-1", "gpt-5.5")
	if params.ServiceTier != "priority" {
		t.Fatalf("serviceTier = %q, want priority", params.ServiceTier)
	}
}

func TestAppServerTurnIncludesReasoningEffortAndServiceTier(t *testing.T) {
	params := buildAppServerTurnParams(harness.RunRequest{
		Preferences: harness.RunPreferences{
			ReasoningEffort: "xhigh",
			ServiceTier:     "default",
		},
	}, "thread-1", "gpt-5.5")
	if params.Effort != "xhigh" || params.ServiceTier != "default" {
		t.Fatalf("turn params = %#v", params)
	}
}

func TestAppServerTurnIgnoresInvalidServiceTier(t *testing.T) {
	params := buildAppServerTurnParams(harness.RunRequest{
		Preferences: harness.RunPreferences{ServiceTier: "turbo;invalid"},
	}, "thread-1", "gpt-5.5")

	if params.ServiceTier != "" {
		t.Fatalf("invalid service tier should not be sent: %#v", params)
	}
}

func TestAppServerTurnNormalizesExecutionPolicies(t *testing.T) {
	params := buildAppServerTurnParams(harness.RunRequest{
		Preferences: harness.RunPreferences{
			ApprovalPolicy: " never ",
			SandboxPolicy:  " readOnly ",
		},
	}, "thread-1", "gpt-5.5")
	if params.ApprovalPolicy != "never" || params.SandboxPolicy.Type != "readOnly" {
		t.Fatalf("turn execution policies = %#v", params)
	}

	params = buildAppServerTurnParams(harness.RunRequest{
		Preferences: harness.RunPreferences{
			ApprovalPolicy: "unknown",
			SandboxPolicy:  "unknown",
		},
	}, "thread-1", "gpt-5.5")
	if params.ApprovalPolicy != "on-request" || params.SandboxPolicy.Type != "workspaceWrite" {
		t.Fatalf("turn execution policy fallbacks = %#v", params)
	}
}

func TestAppServerResumesThread(t *testing.T) {
	request := buildAppServerThreadRequest(harness.RunRequest{ResumeID: "thread-123"})
	if request.Method != "thread/resume" || request.Params.ThreadID != "thread-123" {
		t.Fatalf("thread request = %#v", request)
	}
}

func TestNativePlanTurnUsesCollaborationMode(t *testing.T) {
	params := buildAppServerTurnParams(
		harness.RunRequest{Mode: harness.RunModePlan},
		"thread-123",
		"gpt-5.5",
	)
	if params.CollaborationMode.Mode != harness.RunModePlan {
		t.Fatalf("collaboration mode = %#v", params.CollaborationMode)
	}
	settings := params.CollaborationMode.Settings
	if settings.DeveloperInstructions != nil || settings.ReasoningEffort != nil {
		t.Fatalf("plan settings = %#v", settings)
	}
}

func TestNativePlanTurnUsesSelectedReasoningPreset(t *testing.T) {
	params := buildAppServerTurnParams(
		harness.RunRequest{
			Mode: harness.RunModePlan,
			Preferences: harness.RunPreferences{
				ReasoningEffort: harness.ReasoningEffort("high"),
			},
		},
		"thread-123",
		"gpt-5.5",
	)
	if params.CollaborationMode.Settings.ReasoningEffort == nil ||
		*params.CollaborationMode.Settings.ReasoningEffort != "high" {
		t.Fatalf("plan settings = %#v", params.CollaborationMode.Settings)
	}
}

func TestNativeDefaultTurnUsesProviderInstructions(t *testing.T) {
	params := buildAppServerTurnParams(
		harness.RunRequest{Mode: harness.RunModeDefault},
		"thread-123",
		"gpt-5.5",
	)
	mode := params.CollaborationMode
	if mode.Mode != harness.RunModeDefault {
		t.Fatalf("collaboration mode = %#v", mode)
	}
	settings := mode.Settings
	if settings.DeveloperInstructions != nil || settings.ReasoningEffort != nil {
		t.Fatalf("default settings = %#v", settings)
	}
}

func TestForkUsesNativeThreadFork(t *testing.T) {
	request := buildAppServerThreadRequest(harness.RunRequest{ResumeID: "thread-123", Fork: true})
	if request.Method != "thread/fork" || request.Params.ThreadID != "thread-123" {
		t.Fatalf("thread request = %#v", request)
	}
}
