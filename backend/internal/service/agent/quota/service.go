// Package quota remembers the last subscription-quota reading each provider
// account reports, so the dashboard can show how much of each plan is left.
//
// Readings arrive two ways. Runs report the windows the CLIs mention while
// they work, and Refresh asks every provider that can read plan limits on
// demand, the way Claude Code's /usage and Codex's /status do. Three
// consequences shape this service:
//
//   - A reading is a snapshot, not a live figure. Every reading carries when
//     it was taken and the UI is expected to say so; a stale number presented
//     as current is worse than an empty card, because an operator would plan
//     around it.
//   - An account nobody has read has no reading at all, and that is not an
//     error. It is the honest state, and it is different from "0% used".
//   - A plan belongs to a provider account, not to the provider. Claude and
//     Codex keep several saved accounts and a chat may pin any of them, so
//     each account's windows are kept apart. Which saved accounts still exist
//     is the account service's answer; readings are only keyed by account ID.
//
// It is also not the usage ledger. The ledger knows what this platform spent;
// the plan is spent from everywhere the operator works. Only the vendor knows
// the total, and these readings are the vendor talking.
package quota

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	configconstants "github.com/futrx-com/remote.futrx.com/internal/config/constants"
)

// readErrorLimit bounds the provider error text kept for one account.
const readErrorLimit = 300

// Repository persists readings across restarts.
type Repository interface {
	Load(ctx context.Context) ([]AccountQuota, error)
	Save(ctx context.Context, readings []AccountQuota) error
}

// Service keeps the readings.
type Service struct {
	// recordMu holds one update through its synchronous save, so an older
	// snapshot is never written after a newer one. mu guards readings and
	// readErrors and is released before saving, so readers do not wait for
	// persistence.
	recordMu sync.Mutex
	mu       sync.RWMutex
	readings map[accountKey]AccountQuota
	// readErrors says why the last on-demand read of an account failed. It
	// is not persisted: it only explains what the Usage tab shows now.
	readErrors map[accountKey]string
	store      Repository

	readers         []agent.PlanUsageReader
	refreshInterval time.Duration
	readTimeout     time.Duration
	now             func() time.Time
	// refreshMu guards refreshedAt and refreshing, the read in progress.
	refreshMu   sync.Mutex
	refreshedAt time.Time
	refreshing  chan struct{}
}

// New loads saved readings from store and asks readers for live plan limits
// whenever Refresh finds the last answer out of date.
func New(ctx context.Context, store Repository, readers ...agent.PlanUsageReader) *Service {
	service := &Service{
		readings:        map[accountKey]AccountQuota{},
		readErrors:      map[accountKey]string{},
		store:           store,
		readers:         readers,
		refreshInterval: configconstants.PlanUsageRefreshInterval,
		readTimeout:     configconstants.PlanUsageReadTimeout,
		now:             time.Now,
	}
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
	key := newAccountKey(provider, accountID)
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
	s.saveLocked(ctx)
}

// Refresh asks every provider that can read plan limits on demand for each
// account's current windows. An answer stays current for the refresh
// interval, and callers within it, or while a read is running, share it.
// The read belongs to no single caller: it continues, bounded by the read
// timeout, after a caller whose ctx ends stops waiting for it.
func (s *Service) Refresh(ctx context.Context) {
	if s == nil || len(s.readers) == 0 {
		return
	}
	s.refreshMu.Lock()
	done := s.refreshing
	if done == nil {
		if !s.refreshedAt.IsZero() && s.now().Sub(s.refreshedAt) < s.refreshInterval {
			s.refreshMu.Unlock()
			return
		}
		done = make(chan struct{})
		s.refreshing = done
		go s.readAll(done)
	}
	s.refreshMu.Unlock()
	select {
	case <-done:
	case <-ctx.Done():
	}
}

// readAll asks every reader at once and files each account's answer.
func (s *Service) readAll(done chan struct{}) {
	ctx, cancel := context.WithTimeout(context.Background(), s.readTimeout)
	defer cancel()
	var readers sync.WaitGroup
	for _, reader := range s.readers {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for _, usage := range reader.ReadPlanUsage(ctx) {
				s.replace(context.WithoutCancel(ctx), reader.ID(), usage)
			}
		}()
	}
	readers.Wait()

	s.refreshMu.Lock()
	s.refreshedAt = s.now()
	s.refreshing = nil
	s.refreshMu.Unlock()
	close(done)
}

// replace files one account's live answer. Its windows replace the last
// reading, because the vendor has just said which windows are in effect. A
// failed read keeps the last windows and records why.
func (s *Service) replace(ctx context.Context, provider agent.ProviderID, usage agent.AccountPlanUsage) {
	key := newAccountKey(provider, usage.AccountID)
	if key.provider == "" {
		return
	}

	s.recordMu.Lock()
	defer s.recordMu.Unlock()

	s.mu.Lock()
	if usage.Err != nil {
		s.readErrors[key] = readErrorText(usage.Err)
		s.mu.Unlock()
		return
	}
	delete(s.readErrors, key)
	reading := AccountQuota{Provider: key.provider, AccountID: key.account}
	for _, window := range usage.Windows {
		switch window.Window {
		case agent.QuotaWindowSession:
			reading.Session = cloneWindow(&window)
		case agent.QuotaWindowWeekly:
			reading.Weekly = cloneWindow(&window)
		}
	}
	if reading.Session == nil && reading.Weekly == nil {
		delete(s.readings, key)
	} else {
		s.readings[key] = reading
	}
	s.saveLocked(ctx)
}

// saveLocked releases s.mu, then persists a snapshot of the readings taken
// while it was held. s.mu and s.recordMu must be held by the caller.
func (s *Service) saveLocked(ctx context.Context) {
	if s.store == nil {
		s.mu.Unlock()
		return
	}
	snapshot := s.readingsLocked()
	s.mu.Unlock()
	_ = s.store.Save(ctx, snapshot)
}

// View lists every account with a reading or a failed read, by provider and
// then account, in a stable order so the card does not reshuffle between
// polls.
func (s *Service) View() []AccountView {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]AccountView, 0, len(s.readings)+len(s.readErrors))
	for key, reading := range s.readings {
		out = append(out, AccountView{AccountQuota: reading.clone(), Error: s.readErrors[key]})
	}
	for key, message := range s.readErrors {
		if _, ok := s.readings[key]; !ok {
			out = append(out, AccountView{
				AccountQuota: AccountQuota{Provider: key.provider, AccountID: key.account},
				Error:        message,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Provider != out[j].Provider {
			return out[i].Provider < out[j].Provider
		}
		return out[i].AccountID < out[j].AccountID
	})
	return out
}

// readingsLocked copies every reading, so the repository never shares
// windows with the service. s.mu must be held.
func (s *Service) readingsLocked() []AccountQuota {
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

func newAccountKey(provider agent.ProviderID, accountID string) accountKey {
	return accountKey{
		provider: strings.TrimSpace(string(provider)),
		account:  strings.TrimSpace(accountID),
	}
}

// readErrorText keeps a provider error to one bounded line.
func readErrorText(err error) string {
	message := strings.Join(strings.Fields(err.Error()), " ")
	if runes := []rune(message); len(runes) > readErrorLimit {
		message = string(runes[:readErrorLimit]) + "…"
	}
	return message
}
