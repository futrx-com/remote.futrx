package kimi

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/service/harness"
)

func TestRunRejectsHeadlessPlanMode(t *testing.T) {
	authorizer := &runAccountAuthorizer{allowChecks: 1}
	provider := &Provider{accountAuthorizer: authorizer}
	err := provider.Run(context.Background(), harness.RunRequest{Prompt: "inspect", Mode: harness.RunModePlan}, nil)
	if !errors.Is(err, errHeadlessPlanMode) {
		t.Fatalf("plan request returned %v", err)
	}
}

func TestArgsUsePrintMode(t *testing.T) {
	provider := &Provider{}
	args := provider.args(harness.RunRequest{Prompt: "implement", Mode: harness.RunModeDefault})
	if want := []string{"-p", "implement", "--output-format", "stream-json"}; !slices.Equal(args, want) {
		t.Fatalf("args = %#v, want %#v", args, want)
	}
}

func TestArgsPreserveExactConfiguredModelAlias(t *testing.T) {
	provider := &Provider{}
	args := provider.args(harness.RunRequest{Prompt: "inspect", Model: "moonshot/kimi-k2[1m]"})
	for index := 0; index+1 < len(args); index++ {
		if args[index] == "--model" && args[index+1] == "moonshot/kimi-k2[1m]" {
			return
		}
	}
	t.Fatalf("exact model alias missing: %#v", args)
}
