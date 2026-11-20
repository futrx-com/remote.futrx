package claude

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	harnessruntime "github.com/futrx-com/remote.futrx.com/internal/integration/harnesses/runtime"
	"github.com/futrx-com/remote.futrx.com/internal/service/harness"
)

// queryEffortOptions asks Claude Code's local /effort command for the choices
// available to the current account and configuration. Unlike --help, this
// includes session settings such as Ultracode only when workflows are enabled.
// The command is handled locally and does not invoke a model or consume tokens.
func queryEffortOptions(
	ctx context.Context,
	req harness.CapabilityRequest,
) ([]harness.CapabilityOption, error) {
	cmd := harnessruntime.NewCapabilityCommand(
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
