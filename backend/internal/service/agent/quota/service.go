// Package agentquota remembers the last subscription-quota reading each agent
// CLI volunteered, so the dashboard can show how much of a plan is left.
//
// The whole package exists because this number cannot be asked for. Claude and
// Codex each mention their rolling windows in the middle of a run and offer no
// endpoint, so the platform's only move is to notice what goes past and keep
// it. Two consequences shape everything here:
//
//   - A reading is a snapshot, not a live figure. Every reading carries when
//     it was taken and the UI is expected to say so; a stale number presented
//     as current is worse than an empty card, because an operator would plan
//     around it.
//   - An agent nobody has run has no reading at all, and that is not an error.
//     It is the honest state, and it is different from "0% used".
//
// It is also not the usage ledger. The ledger knows what this platform spent;
// the plan is spent from everywhere the operator works. Only the vendor knows
// the total, and these events are the vendor talking.
package quota

import (
	"context"
	"sort"
	"strings"
	"sync"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
)

// Repository persists readings across restarts.
type Repository interface {
	Load(ctx context.Context) (map[string]AgentQuota, error)
	Save(ctx context.Context, readings map[string]AgentQuota) error
}

// AgentQuota is every window one agent has reported.
type AgentQuota struct {
	Provider string `json:"provider"`
	// Session and Weekly are pointers because "not reported" and "reported
	// as empty" are different states and only one of them should render.
	Session *agent.Quota `json:"session,omitempty"`
	Weekly  *agent.Quota `json:"weekly,omitempty"`
}

// Service keeps the readings.
type Service struct {
	mu       sync.RWMutex
	readings map[string]AgentQuota
	store    Repository
}

func New(ctx context.Context, store Repository) *Service {
	service := &Service{readings: map[string]AgentQuota{}, store: store}
	if store != nil {
		if loaded, err := store.Load(ctx); err == nil && loaded != nil {
			service.readings = loaded
		}
	}
	return service
}

// Record files one reading against one agent.
//
// Persisting is best effort and deliberately not on the run's critical path:
// losing the last reading of a window costs a stale dashboard until the next
// run, and failing a turn over it would cost the operator their actual work.
func (s *Service) Record(ctx context.Context, provider agent.ProviderID, quota agent.Quota) {
	if s == nil || quota.Window == "" {
		return
	}
	id := strings.TrimSpace(string(provider))
	if id == "" {
		return
	}

	s.mu.Lock()
	current := s.readings[id]
	current.Provider = id
	switch quota.Window {
	case agent.QuotaWindowSession:
		current.Session = &quota
	case agent.QuotaWindowWeekly:
		current.Weekly = &quota
	default:
		s.mu.Unlock()
		return
	}
	s.readings[id] = current
	snapshot := s.snapshotLocked()
	s.mu.Unlock()

	if s.store != nil {
		_ = s.store.Save(ctx, snapshot)
	}
}

// View lists what is known, in a stable order so the card does not reshuffle
// between polls.
func (s *Service) View() []AgentQuota {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]AgentQuota, 0, len(s.readings))
	for _, reading := range s.readings {
		out = append(out, reading)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Provider < out[j].Provider })
	return out
}

func (s *Service) snapshotLocked() map[string]AgentQuota {
	out := make(map[string]AgentQuota, len(s.readings))
	for id, reading := range s.readings {
		out[id] = reading
	}
	return out
}
