// Package quota remembers the last subscription-quota reading each provider
// account reports, so the dashboard can show how much of each plan is left.
//
// Integrations obtain quota through provider-specific protocols and submit
// normalized observations here. Three consequences shape this service:
//
//   - A reading is a snapshot, not a live figure. Every reading carries when
//     it was taken and the UI is expected to say so; a stale number presented
//     as current is worse than an empty card, because an operator would plan
//     around it.
//   - An account nobody has run has no reading at all, and that is not an
//     error. It is the honest state, and it is different from "0% used".
//   - A plan belongs to a provider account, not to the provider. Claude and
//     Codex keep several saved accounts and a chat may pin any of them, so
//     each account's windows are kept apart. Which saved accounts still exist
//     is the account service's answer; readings are only keyed by account ID.
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
	Load(ctx context.Context) ([]AccountQuota, error)
	Save(ctx context.Context, readings []AccountQuota) error
}

// Service keeps the readings.
type Service struct {
	// recordMu holds one update through its synchronous save, so an older
	// snapshot is never written after a newer one. mu guards readings and is
	// released before saving, so readers do not wait for persistence.
	recordMu sync.Mutex
	mu       sync.RWMutex
	readings map[accountKey]AccountQuota
	store    Repository
}

func New(ctx context.Context, store Repository) *Service {
	service := &Service{readings: map[accountKey]AccountQuota{}, store: store}
	if store == nil {
		return service
	}
	loaded, err := store.Load(ctx)
	if err != nil {
		return service
	}
	for _, reading := range loaded {
		reading.Provider = strings.TrimSpace(reading.Provider)
		reading.AccountID = strings.TrimSpace(reading.AccountID)
		if reading.Provider == "" {
			continue
		}
		service.readings[reading.key()] = reading.clone()
	}
	return service
}

// Record files one reading against the provider account that reported it. An
// empty accountID stands for the provider's host login.
//
// Persisting is synchronous but best effort. Losing the last reading of a
// window costs a stale dashboard until the next observation, while failing a
// turn over it would cost the operator their actual work.
func (s *Service) Record(ctx context.Context, provider agent.ProviderID, accountID string, quota agent.Quota) {
	if s == nil {
		return
	}
	key := accountKey{
		provider: strings.TrimSpace(string(provider)),
		account:  strings.TrimSpace(accountID),
	}
	if key.provider == "" {
		return
	}
	if quota.Window != agent.QuotaWindowSession && quota.Window != agent.QuotaWindowWeekly {
		return
	}

	s.recordMu.Lock()
	defer s.recordMu.Unlock()

	s.mu.Lock()
	current := s.readings[key]
	current.Provider = key.provider
	current.AccountID = key.account
	switch quota.Window {
	case agent.QuotaWindowSession:
		current.Session = cloneWindow(&quota)
	case agent.QuotaWindowWeekly:
		current.Weekly = cloneWindow(&quota)
	}
	s.readings[key] = current
	var snapshot []AccountQuota
	if s.store != nil {
		snapshot = s.viewLocked()
	}
	s.mu.Unlock()

	if s.store != nil {
		_ = s.store.Save(ctx, snapshot)
	}
}

// View lists what is known by provider and then account, in a stable order so
// the card does not reshuffle between polls.
func (s *Service) View() []AccountQuota {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.viewLocked()
}

// viewLocked copies every reading, so callers never share windows with the
// service. s.mu must be held.
func (s *Service) viewLocked() []AccountQuota {
	out := make([]AccountQuota, 0, len(s.readings))
	for _, reading := range s.readings {
		out = append(out, reading.clone())
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Provider != out[j].Provider {
			return out[i].Provider < out[j].Provider
		}
		return out[i].AccountID < out[j].AccountID
	})
	return out
}
