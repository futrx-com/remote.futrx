package api

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

const containerTimeout = 15 * time.Second

const activeSettings = "/root/.local/share/code-server/User/settings.json"

// The command is fixed: browser input is supplied only on stdin, never used
// as a shell command, container name, path, or program argument.
const writeScript = `
const fs = require("fs");
const path = require("path");
const os = require("os");
const durable = "/workspace/.remote/code-server/settings.json";
const active = "/root/.local/share/code-server/User/settings.json";
const input = JSON.parse(fs.readFileSync(0, "utf8"));
if (!input || typeof input !== "object" || Array.isArray(input)) throw new Error("settings must be an object");
if (input["window.title"] === "${rootPath}") input["window.title"] = os.hostname();
const content = JSON.stringify(input, null, 2) + "\n";
for (const file of [durable, active]) {
  fs.mkdirSync(path.dirname(file), { recursive: true, mode: 0o700 });
  const temporary = file + ".remote-tmp";
  fs.writeFileSync(temporary, content, { mode: 0o600 });
  fs.renameSync(temporary, file);
}
`

func readSettings(container string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), containerTimeout)
	defer cancel()
	output, err := exec.CommandContext(ctx, "lxc", "exec", container, "--", "cat", activeSettings).Output()
	if err != nil {
		return nil, fmt.Errorf("read settings in project container: %w", err)
	}
	return output, nil
}

func writeSettings(container string, content []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), containerTimeout)
	defer cancel()
	command := exec.CommandContext(ctx, "lxc", "exec", container, "--", "node", "-e", writeScript)
	command.Stdin = bytes.NewReader(content)
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("write settings in project container: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}
