package containerio

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"os/exec"
	"strings"

	"futrx.local/catalog/applications/code-server/backend/config"
)

// The command is fixed: browser input is supplied only on stdin, never used
// as a shell command, container name, path, or program argument.
//
//go:embed write-settings.js
var writeScript string

func ReadSettings(container string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), config.ContainerTimeout)
	defer cancel()
	output, err := exec.CommandContext(ctx, "lxc", "exec", container, "--", "cat", config.ActiveSettings).Output()
	if err != nil {
		return nil, fmt.Errorf("read settings in project container: %w", err)
	}
	return output, nil
}

func WriteSettings(container string, content []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), config.ContainerTimeout)
	defer cancel()
	command := exec.CommandContext(ctx, "lxc", "exec", container, "--", "node", "-e", writeScript)
	command.Stdin = bytes.NewReader(content)
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("write settings in project container: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}
