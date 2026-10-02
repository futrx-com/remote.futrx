package config

import (
	"encoding/json"
	"fmt"
	"github.com/futrx-com/remote.futrx.com/internal/agent/provisioning"
	"io"
	"os"
	"path/filepath"
	"sync"
)

// AgentInstructionsStore persists operator additions independently of the checkout.
// Applied instructions remain immutable until the process restarts.
type AgentInstructionsStore struct {
	mu       sync.Mutex
	filename string
	profiles []provisioning.Profile
}

func NewAgentInstructionsStore(filename string, profiles []provisioning.Profile) *AgentInstructionsStore {
	return &AgentInstructionsStore{filename: filename, profiles: profiles}
}
func (s *AgentInstructionsStore) Targets() map[string]string {
	result := map[string]string{}
	for _, p := range s.profiles {
		if p.Instructions != nil {
			result[p.ID] = p.Instructions.Path
		}
	}
	return result
}
func (s *AgentInstructionsStore) Read() (json.RawMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	file, err := os.Open(s.filename)
	if os.IsNotExist(err) {
		return json.RawMessage(`{"global":"","providers":{}}`), nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxAgentInstructionsBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxAgentInstructionsBytes {
		return nil, fmt.Errorf("instructions exceed size limit")
	}
	if _, _, err := composeAgentInstructions(s.filename, "", s.profiles); err != nil {
		return nil, err
	}
	return json.RawMessage(data), nil
}
func (s *AgentInstructionsStore) Write(data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(data) > maxAgentInstructionsBytes {
		return fmt.Errorf("instructions exceed size limit")
	}
	if err := os.MkdirAll(filepath.Dir(s.filename), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(s.filename), ".agent-instructions-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if _, _, err = composeAgentInstructions(f.Name(), "", s.profiles); err != nil {
		return err
	}
	return os.Rename(f.Name(), s.filename)
}
