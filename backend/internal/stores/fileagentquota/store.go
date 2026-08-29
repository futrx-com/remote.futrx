package fileagentquota

// File-backed storage for the last subscription-quota reading each agent CLI
// volunteered, at <dataDir>/agent-quota.json.
//
// Mode 0600 like the other settings stores, though this document holds no
// secret — a percentage and a reset time. The file exists so the dashboard has
// something to show after a restart: readings only arrive during a run, so
// without it an operator who restarts the platform sees an empty card until
// they happen to run an agent.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	serviceagentquota "github.com/futrx-com/remote.futrx.com/internal/service/agentquota"
)

var _ serviceagentquota.Repository = (*Store)(nil)

const fileName = "agent-quota.json"

type Store struct {
	root string
	mu   sync.Mutex
}

func New(dataDir string) (*Store, error) {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	return &Store{root: dataDir}, nil
}

func (s *Store) path() string { return filepath.Join(s.root, fileName) }

// Load returns what was last seen. A missing or unreadable file is an empty
// map, not an error: the readings are a convenience, and refusing to start the
// platform because a cache of percentages will not parse would be absurd.
func (s *Store) Load(ctx context.Context) (map[string]serviceagentquota.AgentQuota, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	raw, err := os.ReadFile(s.path())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]serviceagentquota.AgentQuota{}, nil
		}
		return map[string]serviceagentquota.AgentQuota{}, nil
	}
	if len(raw) == 0 {
		return map[string]serviceagentquota.AgentQuota{}, nil
	}
	readings := map[string]serviceagentquota.AgentQuota{}
	if err := json.Unmarshal(raw, &readings); err != nil {
		return map[string]serviceagentquota.AgentQuota{}, nil
	}
	return readings, nil
}

func (s *Store) Save(ctx context.Context, readings map[string]serviceagentquota.AgentQuota) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(s.root, 0o700); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}
	tmp, err := os.CreateTemp(s.root, ".agent-quota-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp agent quota: %w", err)
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return fmt.Errorf("secure temp agent quota: %w", err)
	}
	encoder := json.NewEncoder(tmp)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(readings); err != nil {
		tmp.Close()
		return fmt.Errorf("write agent quota: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp agent quota: %w", err)
	}
	if err := os.Rename(tmp.Name(), s.path()); err != nil {
		return fmt.Errorf("replace agent quota: %w", err)
	}
	return nil
}
