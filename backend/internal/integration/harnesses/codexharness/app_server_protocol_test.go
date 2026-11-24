package codexharness

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/service/harness"
)

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

func TestThreadRequestsCarryDeveloperInstructions(t *testing.T) {
	for _, req := range []harness.RunRequest{
		{SystemInstructions: "shared"},
		{SystemInstructions: "shared", ResumeID: "thread-123"},
		{SystemInstructions: "shared", ResumeID: "thread-123", Fork: true},
	} {
		request := buildAppServerThreadRequest(req)
		if request.Params.DeveloperInstructions != "shared" {
			t.Fatalf("%s request = %#v, want developer instructions", request.Method, request.Params)
		}
	}
}

func TestThreadRequestOmitsEmptyDeveloperInstructions(t *testing.T) {
	encoded, err := json.Marshal(buildAppServerThreadRequest(harness.RunRequest{}).Params)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "developerInstructions") {
		t.Fatalf("host thread request sent developer instructions: %s", encoded)
	}
}
