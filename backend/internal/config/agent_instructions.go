package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/futrx-com/remote.futrx.com/internal/agent/provisioning"
)

const maxAgentInstructionsBytes = 1024 * 1024

// AgentInstructionProfiles composes immutable provider instruction targets.
// Project-owned instructions remain in the workspace and are read by the CLI;
// this configuration only changes Remote's managed provider-home files.
func AgentInstructionProfiles(filename, hostname string, profiles []provisioning.Profile) ([]provisioning.Profile, []byte, error) {
	var additions struct {
		Global    string            `json:"global"`
		Providers map[string]string `json:"providers"`
	}
	if filename != "" {
		file, err := os.Open(filename)
		if err != nil {
			return nil, nil, fmt.Errorf("open agent instructions: %w", err)
		}
		defer file.Close()
		data, err := io.ReadAll(io.LimitReader(file, maxAgentInstructionsBytes+1))
		if err != nil {
			return nil, nil, fmt.Errorf("read agent instructions: %w", err)
		}
		if len(data) > maxAgentInstructionsBytes {
			return nil, nil, fmt.Errorf("agent instructions exceed %d bytes", maxAgentInstructionsBytes)
		}
		if !bytes.HasPrefix(bytes.TrimSpace(data), []byte("{")) {
			return nil, nil, fmt.Errorf("agent instructions must be a JSON object")
		}
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&additions); err != nil {
			return nil, nil, fmt.Errorf("decode agent instructions: %w", err)
		}
		if err := decoder.Decode(new(any)); err != io.EOF {
			return nil, nil, fmt.Errorf("agent instructions must contain exactly one JSON object")
		}
	}
	known := make(map[string]bool, len(profiles))
	for _, profile := range profiles {
		known[profile.ID] = profile.Instructions != nil
	}
	for provider := range additions.Providers {
		if !known[provider] {
			return nil, nil, fmt.Errorf("agent instructions provider %q has no configured instruction target", provider)
		}
	}
	shared := appendInstructionSection(provisioning.InstructionsTemplate(hostname), "Operator global instructions", additions.Global)
	configured := make([]provisioning.Profile, len(profiles))
	for index, profile := range profiles {
		configured[index] = profile.Clone()
		if configured[index].Instructions != nil {
			configured[index].Instructions.Content = appendInstructionSection(shared, "Provider instructions", additions.Providers[profile.ID])
		}
	}
	return configured, shared, nil
}

func appendInstructionSection(base []byte, heading, addition string) []byte {
	if strings.TrimSpace(addition) == "" {
		return append([]byte(nil), base...)
	}
	return []byte(strings.TrimRight(string(base), "\n") + "\n\n# " + heading + "\n\n" + strings.TrimSpace(addition) + "\n")
}
