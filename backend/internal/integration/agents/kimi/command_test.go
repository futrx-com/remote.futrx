package kimi

import (
	"context"
	"github.com/futrx-com/remote.futrx.com/internal/agent"
	"strings"
	"testing"
)

func TestCommandKeepsPromptAndModelOutOfProcessArguments(t *testing.T) {
	p := &Provider{}
	req := agent.RunRequest{Cwd: t.TempDir(), Prompt: "private user prompt", Model: "moonshot/kimi-k2[1m]", Mode: agent.RunModePlan}
	cmd, _, err := p.buildCmd(context.Background(), req, bridgeArgs(), nil)
	if err != nil {
		t.Fatal(err)
	}
	args := strings.Join(cmd.Args, " ")
	if strings.Contains(args, req.Prompt) || strings.Contains(args, req.Model) || !strings.Contains(args, "--input-type=module") {
		t.Fatal("command did not use private stdio bridge")
	}
}
