package claude

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	agentruntime "github.com/futrx-com/remote.futrx.com/internal/integration/agents/runtime"
)

// queryEffortOptions asks Claude Code's local /effort command for the choices
// available to the current account and configuration. Unlike --help, this
// includes session settings such as Ultracode only when workflows are enabled.
// The command is handled locally and does not invoke a model or consume tokens.
func queryEffortOptions(
	ctx context.Context,
	req agent.CapabilityRequest,
) ([]agent.CapabilityOption, error) {
	cmd := agentruntime.NewCapabilityCommand(
		ctx,
		req,
		[]string{"HOME=/root", "IS_SANDBOX=1"},
		"claude",
		"-p",
		"--no-session-persistence",
		"--output-format", "json",
		"/effort",
	)
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var response modelCommandResponse
	if err := json.Unmarshal(output, &response); err != nil {
		return nil, err
	}
	if response.IsError {
		return nil, errors.New(strings.TrimSpace(response.Result))
	}
	options := parseEffortCommandOptions(response.Result)
	if len(options) == 0 {
		return nil, errors.New("claude effort command did not list available choices")
	}
	return options, nil
}
